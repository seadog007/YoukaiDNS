package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"youkaidns/config"
	"youkaidns/server"
	"youkaidns/stats"
	"youkaidns/web"
)

func main() {
	// Customize flag usage to show double dashes
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  --verbose\n")
		fmt.Fprintf(os.Stderr, "    \tShow all DNS logs\n")
		fmt.Fprintf(os.Stderr, "  --web-listen string\n")
		fmt.Fprintf(os.Stderr, "    \tIP address to listen on for web dashboard (default: localhost) (default \"localhost\")\n")
		fmt.Fprintf(os.Stderr, "  --domain string\n")
		fmt.Fprintf(os.Stderr, "    \tDomain suffix for dynamic records (e.g., example.com)\n")
		fmt.Fprintf(os.Stderr, "  --output-dir string\n")
		fmt.Fprintf(os.Stderr, "    \tDirectory to save received files (default \"received_files\")\n")
	}

	// Parse command-line flags
	verbose := flag.Bool("verbose", false, "Show all DNS logs")
	webListenIP := flag.String("web-listen", "localhost", "IP address to listen on for web dashboard (default: localhost)")
	domain := flag.String("domain", "", "Domain suffix for dynamic records (e.g., example.com)")
	outputDir := flag.String("output-dir", "received_files", "Directory to save received files")
	flag.Parse()

	cfg := config.DefaultConfig()

	// Initialize statistics
	statsCollector := stats.NewStats()

	// Initialize DNS server with verbose flag, domain, and output directory
	dnsServer := server.NewServer(cfg.DNSPort, statsCollector, *verbose, *domain, *outputDir)

	// Initialize web dashboard with listen IP
	webServer := web.NewServer(cfg.WebPort, statsCollector, *webListenIP, dnsServer)

	// Start DNS server
	if err := dnsServer.Start(); err != nil {
		log.Fatalf("Failed to start DNS server: %v", err)
	}

	// Start web dashboard
	go func() {
		if err := webServer.Start(); err != nil {
			log.Fatalf("Failed to start web server: %v", err)
		}
	}()

	log.Println("YoukaiDNS server started")
	log.Printf("DNS server: UDP port %d", cfg.DNSPort)
	if *domain != "" {
		log.Printf("Dynamic records domain: %s", *domain)
	}
	log.Printf("Output directory: %s", *outputDir)
	if *webListenIP == "localhost" || *webListenIP == "127.0.0.1" {
		log.Printf("Web dashboard: http://localhost:%d", cfg.WebPort)
	} else {
		log.Printf("Web dashboard: http://%s:%d", *webListenIP, cfg.WebPort)
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	dnsServer.Stop()
	log.Println("Server stopped")
}
