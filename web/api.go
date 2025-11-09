package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"youkaidns/server"
	"youkaidns/stats"
)

// API handles REST API endpoints
type API struct {
	stats     *stats.Stats
	dnsServer *server.Server
}

// NewAPI creates a new API handler
func NewAPI(s *stats.Stats, dnsServer *server.Server) *API {
	return &API{
		stats:     s,
		dnsServer: dnsServer,
	}
}

// HandleStats returns statistics as JSON
func (a *API) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snapshot := a.stats.GetSnapshot()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(snapshot); err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		return
	}
}

// HandleTransfers returns file transfer progress as JSON
func (a *API) HandleTransfers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if a.dnsServer == nil {
		http.Error(w, "DNS server not available", http.StatusInternalServerError)
		return
	}

	transfers := a.dnsServer.GetFileTransfers()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(transfers); err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		return
	}
}

// HandleFiles returns list of received files as JSON
func (a *API) HandleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if a.dnsServer == nil {
		http.Error(w, "DNS server not available", http.StatusInternalServerError)
		return
	}

	files, err := a.dnsServer.GetReceivedFiles()
	if err != nil {
		http.Error(w, "Error reading files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(files); err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		return
	}
}

// HandleDownload serves a file for download
func (a *API) HandleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if a.dnsServer == nil {
		http.Error(w, "DNS server not available", http.StatusInternalServerError)
		return
	}

	// Get filename from query parameter
	filename := r.URL.Query().Get("file")
	if filename == "" {
		http.Error(w, "Missing file parameter", http.StatusBadRequest)
		return
	}

	// Sanitize filename to prevent directory traversal
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	outputDir := a.dnsServer.GetOutputDir()
	filePath := filepath.Join(outputDir, filename)

	// Check if file exists
	info, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	// Serve the file
	http.ServeFile(w, r, filePath)
}

