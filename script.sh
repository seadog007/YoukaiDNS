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

# Read file and send chunks (1-based part numbering)
PART_NUM=1
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
    echo -n "Sending part $PART_NUM/$TOTAL_PARTS... "
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
echo "Initial transfer complete! Sent $TOTAL_PARTS parts."

# Check for missing chunks and retry
MAX_RETRIES=10
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    # Wait a bit for server to process
    sleep 0.5
    
    # Query for missing chunks
    MISSING_QUERY="missing.${HASH8}.${DOMAIN}"
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
    echo "Found $MISSING_COUNT missing chunk(s): $(echo $MISSING_CHUNKS | tr '\n' ' ')"
    
    # Retry sending missing chunks (1-based, so subtract 1 for offset)
    for chunk_num in $MISSING_CHUNKS; do
        # Calculate byte offset for this chunk (1-based part number, so subtract 1)
        chunk_offset=$(((chunk_num - 1) * CHUNK_SIZE))
        
        # Read chunk and encode to hex
        CHUNK_HEX=$(dd if="$FILE" bs=1 skip=$chunk_offset count=$CHUNK_SIZE 2>/dev/null | xxd -p | tr -d '\n')
        
        # Split hex into DNS labels
        CHUNK_HEX_LABELS=$(split_hex_labels "$CHUNK_HEX")
        
        # Build data record query
        DATA_QUERY="${CHUNK_HEX_LABELS}.${chunk_num}.${HASH8}.${DOMAIN}"
        
        # Send DNS query
        echo -n "  Retrying chunk $chunk_num... "
        if [ -z "$DNS_SERVER" ]; then
            dig +short "$DATA_QUERY" TXT > /dev/null 2>&1 || true
        else
            dig +short @"$DNS_SERVER" "$DATA_QUERY" TXT > /dev/null 2>&1 || true
        fi
        echo "done"
        
        sleep 0.05
    done
    
    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo "Retry attempt $RETRY_COUNT/$MAX_RETRIES completed"
    echo ""
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "Warning: Reached maximum retry attempts. Some chunks may still be missing."
    # Final check
    if [ -z "$DNS_SERVER" ]; then
        FINAL_CHECK=$(dig +short "missing.${HASH8}.${DOMAIN}" TXT 2>/dev/null || echo "")
    else
        FINAL_CHECK=$(dig +short @"$DNS_SERVER" "missing.${HASH8}.${DOMAIN}" TXT 2>/dev/null || echo "")
    fi
    if [ -n "$FINAL_CHECK" ]; then
        echo "Still missing: $(echo "$FINAL_CHECK" | grep -oE '"[0-9]+"' | tr -d '"' | tr '\n' ' ')"
    fi
fi

echo ""
echo "Transfer complete! File should be received as: $FILENAME"

