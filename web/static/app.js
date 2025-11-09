// Update interval in milliseconds
const UPDATE_INTERVAL = 2000;

let updateTimer = null;

// Format duration in milliseconds
function formatDuration(durationMs) {
    if (durationMs < 1) {
        return (durationMs * 1000).toFixed(2) + 'μs';
    } else if (durationMs < 1000) {
        return durationMs.toFixed(2) + 'ms';
    } else {
        return (durationMs / 1000).toFixed(2) + 's';
    }
}

// Format Go duration string (e.g., "123.456ms") to milliseconds
function parseDuration(durationStr) {
    if (!durationStr || durationStr === '0s') {
        return 0;
    }
    
    // Remove 'ms', 's', 'ns', 'µs' (micro sign) or 'μs' (Greek mu) and convert
    // Go uses 'µs' (U+00B5 micro sign), but we handle both for compatibility
    if (durationStr.endsWith('ms')) {
        return parseFloat(durationStr.replace('ms', ''));
    } else if (durationStr.endsWith('s') && !durationStr.endsWith('µs') && !durationStr.endsWith('μs') && !durationStr.endsWith('ns')) {
        return parseFloat(durationStr.replace('s', '')) * 1000;
    } else if (durationStr.endsWith('µs') || durationStr.endsWith('μs')) {
        // Handle both micro sign (U+00B5) and Greek mu (U+03BC)
        const value = parseFloat(durationStr.replace(/[µμ]s$/, ''));
        return value / 1000; // Convert microseconds to milliseconds
    } else if (durationStr.endsWith('ns')) {
        return parseFloat(durationStr.replace('ns', '')) / 1000000;
    }
    return 0;
}

// Update the dashboard with new stats
function updateDashboard(data) {
    // Update main stats
    document.getElementById('total-queries').textContent = data.total_queries.toLocaleString();
    document.getElementById('successful-resps').textContent = data.successful_responses.toLocaleString();
    document.getElementById('failed-resps').textContent = data.failed_responses.toLocaleString();
    
    // Update response time
    const avgMs = parseDuration(data.response_time.avg);
    document.getElementById('avg-response-time').textContent = formatDuration(avgMs);
    
    // Update queries by type
    const typeContainer = document.getElementById('queries-by-type');
    typeContainer.innerHTML = '';
    
    if (Object.keys(data.queries_by_type).length === 0) {
        typeContainer.innerHTML = '<p style="color: #999; text-align: center; padding: 20px;">No queries yet</p>';
    } else {
        const sortedTypes = Object.entries(data.queries_by_type)
            .sort((a, b) => b[1] - a[1]);
        
        sortedTypes.forEach(([type, count]) => {
            const item = document.createElement('div');
            item.className = 'type-item';
            item.innerHTML = `
                <span class="type-label">${type}</span>
                <span class="type-count">${count.toLocaleString()}</span>
            `;
            typeContainer.appendChild(item);
        });
    }
    
    // Update response time statistics
    const minMs = parseDuration(data.response_time.min);
    const maxMs = parseDuration(data.response_time.max);
    const avgMs2 = parseDuration(data.response_time.avg);
    
    document.getElementById('min-time').textContent = formatDuration(minMs);
    document.getElementById('max-time').textContent = formatDuration(maxMs);
    document.getElementById('avg-time').textContent = formatDuration(avgMs2);
    document.getElementById('time-count').textContent = data.response_time.count.toLocaleString();
    
    // Update last update time
    document.getElementById('last-update').textContent = new Date().toLocaleTimeString();
}

// Update file transfers display
function updateTransfers(transfers) {
    const container = document.getElementById('file-transfers');
    container.innerHTML = '';

    if (!transfers || transfers.length === 0) {
        container.innerHTML = '<p style="color: #999; text-align: center; padding: 20px;">No active file transfers</p>';
        return;
    }

    transfers.forEach(transfer => {
        const transferDiv = document.createElement('div');
        transferDiv.className = `transfer-item ${transfer.status}`;

        const progress = transfer.progress || 0;
        const receivedParts = transfer.received_parts || 0;
        const totalParts = transfer.total_parts || 0;
        const missingChunks = transfer.missing_chunks || [];

        transferDiv.innerHTML = `
            <div class="transfer-header">
                <div class="transfer-filename">${escapeHtml(transfer.filename || 'Unknown')}</div>
                <div class="transfer-status ${transfer.status}">${transfer.status}</div>
            </div>
            <div class="transfer-info">
                <div class="transfer-info-item">
                    <span class="transfer-info-label">Hash</span>
                    <span class="transfer-info-value">${transfer.hash || 'N/A'}</span>
                </div>
                <div class="transfer-info-item">
                    <span class="transfer-info-label">File Size</span>
                    <span class="transfer-info-value">${transfer.total_bytes > 0 ? formatFileSize(transfer.total_bytes) : 'Unknown'}</span>
                </div>
                <div class="transfer-info-item">
                    <span class="transfer-info-label">Progress</span>
                    <span class="transfer-info-value">${receivedParts} / ${totalParts} chunks</span>
                </div>
                <div class="transfer-info-item">
                    <span class="transfer-info-label">Chunk Size</span>
                    <span class="transfer-info-value">${transfer.chunk_size || 0} bytes</span>
                </div>
                <div class="transfer-info-item">
                    <span class="transfer-info-label">Percentage</span>
                    <span class="transfer-info-value">${progress.toFixed(1)}%</span>
                </div>
            </div>
            <div class="progress-bar-container">
                <div class="progress-bar ${transfer.status}" style="width: ${progress}%">
                    ${progress >= 10 ? progress.toFixed(1) + '%' : ''}
                </div>
            </div>
            ${missingChunks.length > 0 ? `
                <div class="missing-chunks">
                    <span class="missing-chunks-label">Missing chunks:</span>
                    <span class="missing-chunks-list">${missingChunks.slice(0, 20).join(', ')}${missingChunks.length > 20 ? '...' : ''}</span>
                </div>
            ` : ''}
        `;

        container.appendChild(transferDiv);
    });
}

// Escape HTML to prevent XSS
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Fetch stats from API
async function fetchStats() {
    try {
        const response = await fetch('/api/stats');
        if (!response.ok) {
            throw new Error('Failed to fetch stats');
        }
        const data = await response.json();
        updateDashboard(data);
    } catch (error) {
        console.error('Error fetching stats:', error);
        // Show error message
        const errorMsg = document.createElement('div');
        errorMsg.style.cssText = 'position: fixed; top: 20px; right: 20px; background: #ff4444; color: white; padding: 15px; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.2);';
        errorMsg.textContent = 'Error fetching stats. Retrying...';
        document.body.appendChild(errorMsg);
        setTimeout(() => errorMsg.remove(), 3000);
    }
}

// Fetch file transfers from API
async function fetchTransfers() {
    try {
        const response = await fetch('/api/transfers');
        if (!response.ok) {
            throw new Error('Failed to fetch transfers');
        }
        const data = await response.json();
        updateTransfers(data);
    } catch (error) {
        console.error('Error fetching transfers:', error);
    }
}

// Format file size
function formatFileSize(bytes) {
    if (bytes < 1024) {
        return bytes + ' B';
    } else if (bytes < 1024 * 1024) {
        return (bytes / 1024).toFixed(2) + ' KB';
    } else if (bytes < 1024 * 1024 * 1024) {
        return (bytes / (1024 * 1024)).toFixed(2) + ' MB';
    } else {
        return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
    }
}

// Format date
function formatDate(dateStr) {
    const date = new Date(dateStr);
    return date.toLocaleString();
}

// Update received files display
function updateFiles(files) {
    const container = document.getElementById('received-files');
    container.innerHTML = '';

    if (!files || files.length === 0) {
        container.innerHTML = '<p style="color: #999; text-align: center; padding: 20px;">No files received yet</p>';
        return;
    }

    files.forEach(file => {
        const fileDiv = document.createElement('div');
        fileDiv.className = 'file-item';

        const fileName = escapeHtml(file.name || 'Unknown');
        const fileSize = file.size || 0;
        const modTime = file.mod_time || '';

        fileDiv.innerHTML = `
            <div class="file-info">
                <div class="file-name">${fileName}</div>
                <div class="file-meta">
                    <span class="file-size">${formatFileSize(fileSize)}</span>
                    <span class="file-date">${formatDate(modTime)}</span>
                </div>
            </div>
            <a href="/api/download?file=${encodeURIComponent(fileName)}" class="download-btn" download="${fileName}">
                Download
            </a>
        `;

        container.appendChild(fileDiv);
    });
}

// Fetch received files from API
async function fetchFiles() {
    try {
        const response = await fetch('/api/files');
        if (!response.ok) {
            throw new Error('Failed to fetch files');
        }
        const data = await response.json();
        updateFiles(data);
    } catch (error) {
        console.error('Error fetching files:', error);
    }
}

// Start auto-refresh
function startAutoRefresh() {
    fetchStats(); // Initial fetch
    fetchTransfers(); // Initial fetch
    fetchFiles(); // Initial fetch
    updateTimer = setInterval(() => {
        fetchStats();
        fetchTransfers();
        fetchFiles();
    }, UPDATE_INTERVAL);
}

// Stop auto-refresh
function stopAutoRefresh() {
    if (updateTimer) {
        clearInterval(updateTimer);
        updateTimer = null;
    }
}

// Initialize on page load
document.addEventListener('DOMContentLoaded', () => {
    startAutoRefresh();
});

// Cleanup on page unload
window.addEventListener('beforeunload', () => {
    stopAutoRefresh();
});

