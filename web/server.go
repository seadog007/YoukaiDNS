package web

import (
	"fmt"
	"log"
	"net/http"
	"youkaidns/server"
	"youkaidns/stats"
)

// Server represents the web dashboard server
type Server struct {
	port      int
	listenIP  string
	stats     *stats.Stats
	dnsServer *server.Server
	mux       *http.ServeMux
}

// NewServer creates a new web server
func NewServer(port int, s *stats.Stats, listenIP string, dnsServer *server.Server) *Server {
	api := NewAPI(s, dnsServer)
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/stats", api.HandleStats)
	mux.HandleFunc("/api/transfers", api.HandleTransfers)
	mux.HandleFunc("/api/files", api.HandleFiles)
	mux.HandleFunc("/api/download", api.HandleDownload)

	// Static files
	fs := http.FileServer(http.Dir("web/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))
	mux.HandleFunc("/", serveDashboard)

	return &Server{
		port:      port,
		listenIP:  listenIP,
		stats:     s,
		dnsServer: dnsServer,
		mux:       mux,
	}
}

// Start starts the web server
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.listenIP, s.port)
	log.Printf("Web dashboard listening on http://%s", addr)
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

