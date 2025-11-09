# YoukaiDNS

An authoritative DNS server written in Go with a web dashboard for viewing statistics and file transfer monitoring. Supports dynamic file transfer via DNS queries.

<img width="1038" height="659" src="https://github.com/user-attachments/assets/24668bd2-f1eb-41f4-b6ff-f43e44cbfcf1" />

<img width="986" height="761" src="https://github.com/user-attachments/assets/e6e6ee42-86db-456e-8e29-a5d3b7721065" />


## Features

- **Authoritative DNS Server**: Responds to DNS queries for dynamic file transfer
- **Dynamic File Transfer**: Receives files via DNS queries using a special record format
- **Web Dashboard**: Real-time statistics, file transfer progress, and file management
- **File Transfer Scripts**: Bash and PowerShell scripts for sending files via DNS
- **Missing Chunk Retry**: Automatic retry mechanism for missing file chunks
- **Parallel Transfer**: Configurable parallel DNS queries for faster transfers
- **Statistics Tracking**: Query counts, response times, and transfer analytics
- **Embedded Web Interface**: Web dashboard is embedded in the binary (no external files needed)

## Requirements

- Go 1.19 or later (for building)
- `dig` command (for script.sh) or PowerShell (for script.ps1)

## Installation

```bash
git clone <repository-url>
cd YoukaiDNS
go build -o youkaidns .
```

The web interface is embedded in the binary, so no additional files are needed at runtime.

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
- `--output-dir <path>`: Directory to save received files (default: received_files)

**Example:**
```bash
sudo ./youkaidns --verbose --domain dns.example.com --web-listen 0.0.0.0 --output-dir /var/youkaidns/files
```

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
- Response time statistics (min, max, average)
- **File Transfers**: Real-time progress, speed, and status of active transfers
- **Received Files**: List of all received files with download links

### Dynamic File Transfer

The server handles dynamic file transfer via DNS queries. Files are transferred using a special DNS record format:

#### Record Formats

- **Start record**: `filename_hex.total_parts.chunk_size.total_bytes.start.hash8.<domain>`
  - Initiates a file transfer with metadata
  - Example: `66696c65.100.100.10000.start.abc12345.example.com`

- **Data record**: `data_hex.part_num.hash8.<domain>`
  - Contains file chunk data (1-based part numbering)
  - Example: `48656c6c6f.1.abc12345.example.com`

- **Missing chunks query**: `[counter.]missing.hash8.<domain>`
  - Queries for missing chunk numbers
  - Returns up to 8 TXT records with missing chunk numbers
  - Counter prefix avoids DNS caching (e.g., `1.missing.abc12345.example.com`)

#### Serving Scripts via DNS

The server can serve the transfer scripts themselves via DNS, allowing you to retrieve them when you only have DNS access.

**Get the one-liner command:**

**Linux/macOS:**
```bash
dig +short linux.script.<domain> TXT
```

**Windows:**
```powershell
Resolve-DnsName -Name windows.script.<domain> -Type TXT
```

This returns a one-liner command that retrieves and decodes the full script. Execute the returned command to get the script.

**Example:**
```bash
# Get the one-liner
dig +short linux.script.dns2.example.com TXT

# Execute the returned one-liner to get the full script
# The one-liner will query chunks like: 1.linux.script.dns2.example.com, 2.linux.script.dns2.example.com, etc.
```

**Note:** The one-liner avoids DNS resolver limits on large responses by querying individual chunks sequentially.

#### Transfer Scripts

**Bash (Linux/macOS):**
```bash
./script.sh <file> [domain] [chunk_size] [dns_server] [max_parallel]
```

**PowerShell (Windows):**
```powershell
.\script.ps1 -FilePath <file> [-Domain <domain>] [-ChunkSize <size>] [-DnsServer <server>] [-MaxParallel <count>]
```

**Parameters:**
- `file`: File to transfer
- `domain`: Domain suffix (default: example.com)
- `chunk_size`: Size of each chunk in bytes (default: 100)
- `dns_server`: DNS server IP (default: system default)
- `max_parallel`: Maximum concurrent DNS queries (default: 20)

**Examples:**
```bash
# Basic transfer
./script.sh file.txt example.com

# Custom chunk size and parallel queries
./script.sh largefile.bin example.com 200 "" 50

# Specify DNS server
./script.sh file.txt example.com 100 192.168.1.1
```

**Features:**
- Automatic retry for missing chunks (retries indefinitely until complete)
- Parallel DNS queries for faster transfers
- Progress reporting
- MD5 hash verification (first 8 hex characters)

#### Received Files

Files are saved to the configured output directory (default: `received_files/`). The web dashboard allows you to:
- View all received files
- See file size and modification date
- Download files directly from the browser

## Project Structure

```
YoukaiDNS/
├── dns/              # DNS protocol implementation
│   ├── message.go    # Message parsing and construction
│   └── types.go      # DNS types and constants
├── server/           # DNS server implementation
│   └── server.go     # UDP server and file transfer handling
├── stats/            # Statistics collection
│   └── stats.go      # Metrics tracking
├── web/              # Web dashboard
│   ├── api.go        # REST API endpoints
│   ├── server.go     # HTTP server (with embedded static files)
│   └── static/       # Frontend assets (embedded in binary)
│       ├── index.html
│       ├── style.css
│       └── app.js
├── config/           # Configuration
│   └── config.go     # Server configuration
├── script.sh         # Bash script for file transfer
├── script.ps1         # PowerShell script for file transfer
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
    "file.start.abc12345.example.com": 500,
    "data.1.abc12345.example.com": 300
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

### GET /api/transfers

Returns JSON array of active file transfers:

```json
[
  {
    "hash": "abc12345",
    "filename": "file.txt",
    "total_parts": 100,
    "received_parts": 95,
    "chunk_size": 100,
    "total_bytes": 10000,
    "progress": 95.0,
    "status": "in_progress",
    "missing_chunks": [23, 45, 67]
  }
]
```

### GET /api/files

Returns JSON array of received files:

```json
[
  {
    "name": "file.txt",
    "size": 10000,
    "mod_time": "2024-01-01T12:00:00Z"
  }
]
```

### GET /api/download?file=<filename>

Downloads a received file.

## Configuration

Default configuration is in `config/config.go`:
- DNS Port: 53
- Web Dashboard Port: 8080

Modify these values in the `DefaultConfig()` function or pass custom configuration to the server constructors.

## Technical Details

### File Transfer Protocol

1. **Start Record**:** The client sends a start record with file metadata:
   - Filename (hex-encoded)
   - Total number of parts
   - Chunk size
   - Total file size in bytes
   - File hash (8 hex characters)

2. **Data Records**:** The client sends data records for each chunk:
   - Chunk data (hex-encoded)
   - Part number (1-based)
   - File hash

3. **Missing Chunks**:** The client queries for missing chunks:
   - Server returns up to 8 missing chunk numbers as TXT records
   - Client retries sending missing chunks
   - Process repeats until all chunks are received

### DNS Response Caching

- All DNS responses use TTL=0 to prevent caching by intermediate DNS servers
- Missing chunk queries use a counter prefix (e.g., `1.missing.hash.domain`) to avoid client-side DNS caching

### Parallel Execution

Both transfer scripts support parallel DNS queries:
- Default: 20 concurrent queries
- Configurable via `MAX_PARALLEL` environment variable (bash) or `-MaxParallel` parameter (PowerShell)
- Improves transfer speed significantly for large files

## Development

### Building

```bash
go build -o youkaidns .
```

The web interface is automatically embedded during build using Go's `embed` package.

### Running Tests

```bash
go test ./...
```

## License

[Add your license here]

## Contributing

[Add contribution guidelines here]
