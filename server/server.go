package server

import (
	"fmt"
	"log"
	"net"
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

// Server represents a DNS server
type Server struct {
	port     int
	stats    *stats.Stats
	conn     *net.UDPConn
	shutdown chan struct{}
	
	// Zone data (dynamically generated)
	mu      sync.RWMutex
	records map[string]map[uint16][]Record // domain -> type -> records
}

// NewServer creates a new DNS server
func NewServer(port int, s *stats.Stats) *Server {
	return &Server{
		port:     port,
		stats:    s,
		shutdown: make(chan struct{}),
		records:  make(map[string]map[uint16][]Record),
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
			TTL:   300,
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
}

