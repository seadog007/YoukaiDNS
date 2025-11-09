package config

// Config holds server configuration
type Config struct {
	DNSPort  int // DNS server port (default 53)
	WebPort  int // Web dashboard port (default 8080)
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		DNSPort: 53,
		WebPort: 8080,
	}
}

