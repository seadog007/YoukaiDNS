package web

import (
	"fmt"
	"log"
	"net/http"
	"youkaidns/stats"
)

// Server represents the web dashboard server
type Server struct {
	port  int
	stats *stats.Stats
	mux   *http.ServeMux
}

// NewServer creates a new web server
func NewServer(port int, s *stats.Stats) *Server {
	api := NewAPI(s)
	mux := http.NewServeMux()

	// API endpoint
	mux.HandleFunc("/api/stats", api.HandleStats)

	// Static files
	fs := http.FileServer(http.Dir("web/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))
	mux.HandleFunc("/", serveDashboard)

	return &Server{
		port:  port,
		stats: s,
		mux:   mux,
	}
}

// Start starts the web server
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("Web dashboard listening on http://localhost%s", addr)
	return http.ListenAndServe(addr, s.mux)
}

// serveDashboard serves the dashboard HTML
func serveDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, "web/static/index.html")
}

