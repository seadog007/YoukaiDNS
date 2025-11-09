package web

import (
	"encoding/json"
	"net/http"
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

