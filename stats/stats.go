package stats

import (
	"sync"
	"sync/atomic"
	"time"
)

// Stats holds DNS server statistics
type Stats struct {
	mu sync.RWMutex

	// Atomic counters
	TotalQueries    int64
	QueriesByType   map[uint16]int64 // Type -> count
	QueriesByDomain map[string]int64 // Domain -> count
	SuccessfulResps int64
	FailedResps     int64

	// Response time tracking
	responseTimes []time.Duration
	maxTimes      int // Maximum number of times to keep
}

// NewStats creates a new stats collector
func NewStats() *Stats {
	return &Stats{
		QueriesByType:   make(map[uint16]int64),
		QueriesByDomain: make(map[string]int64),
		responseTimes:   make([]time.Duration, 0, 1000),
		maxTimes:        1000,
	}
}

// RecordQuery records a DNS query
func (s *Stats) RecordQuery(domain string, queryType uint16) {
	atomic.AddInt64(&s.TotalQueries, 1)

	s.mu.Lock()
	s.QueriesByType[queryType]++
	s.QueriesByDomain[domain]++
	s.mu.Unlock()
}

// RecordResponse records a response (success or failure)
func (s *Stats) RecordResponse(success bool, duration time.Duration) {
	if success {
		atomic.AddInt64(&s.SuccessfulResps, 1)
	} else {
		atomic.AddInt64(&s.FailedResps, 1)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Add response time
	s.responseTimes = append(s.responseTimes, duration)
	if len(s.responseTimes) > s.maxTimes {
		// Keep only the most recent times
		s.responseTimes = s.responseTimes[len(s.responseTimes)-s.maxTimes:]
	}
}

// Snapshot returns a snapshot of current statistics
type Snapshot struct {
	TotalQueries    int64              `json:"total_queries"`
	QueriesByType   map[string]int64   `json:"queries_by_type"`
	QueriesByDomain map[string]int64   `json:"queries_by_domain"`
	SuccessfulResps int64              `json:"successful_responses"`
	FailedResps     int64              `json:"failed_responses"`
	ResponseTime    ResponseTimeStats  `json:"response_time"`
}

// ResponseTimeStats holds response time statistics
type ResponseTimeStats struct {
	Min    string `json:"min"`
	Max    string `json:"max"`
	Avg    string `json:"avg"`
	Count  int    `json:"count"`
}

// GetSnapshot returns a snapshot of current statistics
func (s *Stats) GetSnapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := Snapshot{
		TotalQueries:    atomic.LoadInt64(&s.TotalQueries),
		QueriesByType:   make(map[string]int64),
		QueriesByDomain: make(map[string]int64),
		SuccessfulResps: atomic.LoadInt64(&s.SuccessfulResps),
		FailedResps:     atomic.LoadInt64(&s.FailedResps),
	}

	// Copy queries by type
	for k, v := range s.QueriesByType {
		typeName := getTypeName(k)
		snapshot.QueriesByType[typeName] = v
	}

	// Copy top domains (limit to top 10)
	domainCounts := make([]struct {
		domain string
		count  int64
	}, 0, len(s.QueriesByDomain))
	for domain, count := range s.QueriesByDomain {
		domainCounts = append(domainCounts, struct {
			domain string
			count  int64
		}{domain, count})
	}

	// Sort and take top 10 (simple approach)
	for i := 0; i < len(domainCounts) && i < 10; i++ {
		maxIdx := i
		for j := i + 1; j < len(domainCounts); j++ {
			if domainCounts[j].count > domainCounts[maxIdx].count {
				maxIdx = j
			}
		}
		domainCounts[i], domainCounts[maxIdx] = domainCounts[maxIdx], domainCounts[i]
		snapshot.QueriesByDomain[domainCounts[i].domain] = domainCounts[i].count
	}

	// Calculate response time stats
	if len(s.responseTimes) > 0 {
		var sum time.Duration
		minTime := s.responseTimes[0]
		maxTime := s.responseTimes[0]

		for _, rt := range s.responseTimes {
			sum += rt
			if rt < minTime {
				minTime = rt
			}
			if rt > maxTime {
				maxTime = rt
			}
		}

		snapshot.ResponseTime.Count = len(s.responseTimes)
		if snapshot.ResponseTime.Count > 0 {
			avgTime := sum / time.Duration(snapshot.ResponseTime.Count)
			snapshot.ResponseTime.Min = minTime.String()
			snapshot.ResponseTime.Max = maxTime.String()
			snapshot.ResponseTime.Avg = avgTime.String()
		}
	}

	return snapshot
}

// getTypeName returns a string representation of a DNS type
func getTypeName(t uint16) string {
	switch t {
	case 1:
		return "A"
	case 16:
		return "TXT"
	default:
		return "UNKNOWN"
	}
}

