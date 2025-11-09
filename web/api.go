package web

import (
	"encoding/json"
	"net/http"
	"youkaidns/stats"
)

// API handles REST API endpoints
type API struct {
	stats *stats.Stats
}

// NewAPI creates a new API handler
func NewAPI(s *stats.Stats) *API {
	return &API{
		stats: s,
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

