# YoukaiDNS

An authoritative DNS server written in Go with a web dashboard for viewing statistics. Supports A and TXT record types with dynamic zone generation.

## Features

- **Authoritative DNS Server**: Responds to DNS queries for configured domains
- **A Record Support**: IPv4 address resolution
- **TXT Record Support**: Text string records
- **Dynamic Zone Management**: Add records programmatically at runtime
- **Web Dashboard**: Real-time statistics and monitoring
- **Statistics Tracking**: Query counts, response times, and domain analytics

## Requirements

- Go 1.19 or later

## Installation

```bash
git clone <repository-url>
cd YoukaiDNS
go build -o youkaidns .
```

## Usage

### Running the Server

```bash
sudo ./youkaidns
```

**Note**: Running on port 53 (default DNS port) requires root/administrator privileges. You can modify the port in `config/config.go` if you don't have root access.

The server will start:
- DNS server on UDP port 53
- Web dashboard on HTTP port 8080

### Accessing the Dashboard

Open your browser and navigate to:
```
http://localhost:8080
```

The dashboard displays:
- Total queries received
- Successful vs failed responses
- Average response time
- Queries by record type (A, TXT)
- Top queried domains
- Response time statistics (min, max, average)

### Adding Records Programmatically

Records are added dynamically in `main.go`. Example:

```go
// Add A record
dnsServer.AddRecord("example.com", dns.TypeA, "192.168.1.100")

// Add TXT record
dnsServer.AddRecord("example.com", dns.TypeTXT, []string{"v=spf1 include:_spf.example.com ~all"})
```

### Testing the DNS Server

You can test the DNS server using `dig` or `nslookup`:

```bash
# Query A record
dig @127.0.0.1 example.com A

# Query TXT record
dig @127.0.0.1 example.com TXT
```

Or using `nslookup`:

```bash
nslookup example.com 127.0.0.1
```

## Project Structure

```
YoukaiDNS/
├── dns/              # DNS protocol implementation
│   ├── message.go    # Message parsing and construction
│   └── types.go      # DNS types and constants
├── server/           # DNS server implementation
│   └── server.go     # UDP server and zone management
├── stats/            # Statistics collection
│   └── stats.go      # Metrics tracking
├── web/              # Web dashboard
│   ├── api.go        # REST API endpoints
│   ├── server.go     # HTTP server
│   └── static/       # Frontend assets
│       ├── index.html
│       ├── style.css
│       └── app.js
├── config/           # Configuration
│   └── config.go     # Server configuration
├── main.go           # Entry point
└── README.md         # This file
```

## API Endpoints

### GET /api/stats

Returns JSON statistics:

```json
{
  "total_queries": 1234,
  "queries_by_type": {
    "A": 800,
    "TXT": 434
  },
  "queries_by_domain": {
    "example.com": 500,
    "test.example.com": 300
  },
  "successful_responses": 1200,
  "failed_responses": 34,
  "response_time": {
    "min": "100μs",
    "max": "5ms",
    "avg": "500μs",
    "count": 1200
  }
}
```

## Configuration

Default configuration is in `config/config.go`:
- DNS Port: 53
- Web Dashboard Port: 8080

Modify these values in the `DefaultConfig()` function or pass custom configuration to the server constructors.

## Development

### Building

```bash
go build -o youkaidns .
```

### Running Tests

```bash
go test ./...
```

## License

[Add your license here]

## Contributing

[Add contribution guidelines here]

