package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"youkaidns/config"
	"youkaidns/dns"
	"youkaidns/server"
	"youkaidns/stats"
	"youkaidns/web"
)

func main() {
	cfg := config.DefaultConfig()
	
	// Initialize statistics
	statsCollector := stats.NewStats()

	// Initialize DNS server
	dnsServer := server.NewServer(cfg.DNSPort, statsCollector)

	// Add some sample records (these can be dynamically generated)
	// A records
	dnsServer.AddRecord("example.com", dns.TypeA, "192.168.1.100")
	dnsServer.AddRecord("test.example.com", dns.TypeA, "192.168.1.101")
	dnsServer.AddRecord("www.example.com", dns.TypeA, "192.168.1.102")

	// TXT records
	dnsServer.AddRecord("example.com", dns.TypeTXT, []string{"v=spf1 include:_spf.example.com ~all"})
	dnsServer.AddRecord("test.example.com", dns.TypeTXT, []string{"test-value", "another-value"})

	// Initialize web dashboard
	webServer := web.NewServer(cfg.WebPort, statsCollector)

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
	log.Printf("Web dashboard: http://localhost:%d", cfg.WebPort)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	dnsServer.Stop()
	log.Println("Server stopped")
}

