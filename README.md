# YoukaiDNS

An authoritative DNS server written in Go with a web dashboard for viewing statistics. Supports A and TXT record types with dynamic zone generation.

## Features

- **Authoritative DNS Server**: Responds to DNS queries for dynamic file transfer
- **Dynamic File Transfer**: Receives files via DNS queries using dynamic record format
- **A Record Support**: IPv4 address resolution
- **TXT Record Support**: Text string records
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
sudo ./youkaidns --domain example.com
```

**Note**: Running on port 53 (default DNS port) requires root/administrator privileges. You can modify the port in `config/config.go` if you don't have root access.

**Command-line options:**
- `--verbose`: Show all DNS logs
- `--web-listen <ip>`: IP address to listen on for web dashboard (default: localhost)
- `--domain <domain>`: Domain suffix for dynamic records (e.g., example.com)

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

### Dynamic File Transfer

The server handles dynamic file transfer via DNS queries. Files are transferred using a special DNS record format:

- **Start record**: `filename_hex.total_parts.chunk_size.start.hash8.arg1.<domain>`
- **Data record**: `data_hex.part_num.hash8.arg1.<domain>`

Received files are saved to the `received_files/` directory.

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
    "file.start.abc12345.arg.example.com": 500,
    "data.0.abc12345.arg.example.com": 300
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

