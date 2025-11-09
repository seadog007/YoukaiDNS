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
    
    // Remove 'ms', 's', 'ns', 'μs' and convert
    if (durationStr.endsWith('ms')) {
        return parseFloat(durationStr.replace('ms', ''));
    } else if (durationStr.endsWith('s')) {
        return parseFloat(durationStr.replace('s', '')) * 1000;
    } else if (durationStr.endsWith('μs')) {
        return parseFloat(durationStr.replace('μs', '')) / 1000;
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
    
    // Update top domains
    const domainContainer = document.getElementById('top-domains');
    domainContainer.innerHTML = '';
    
    if (Object.keys(data.queries_by_domain).length === 0) {
        domainContainer.innerHTML = '<p style="color: #999; text-align: center; padding: 20px;">No domains queried yet</p>';
    } else {
        const sortedDomains = Object.entries(data.queries_by_domain)
            .sort((a, b) => b[1] - a[1])
            .slice(0, 10); // Top 10
        
        sortedDomains.forEach(([domain, count]) => {
            const item = document.createElement('div');
            item.className = 'domain-item';
            item.innerHTML = `
                <span class="domain-label">${domain}</span>
                <span class="domain-count">${count.toLocaleString()}</span>
            `;
            domainContainer.appendChild(item);
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

// Start auto-refresh
function startAutoRefresh() {
    fetchStats(); // Initial fetch
    updateTimer = setInterval(fetchStats, UPDATE_INTERVAL);
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

