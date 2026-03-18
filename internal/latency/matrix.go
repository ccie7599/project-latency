package latency

import (
	"sync"
	"time"
)

// Result is a single latency measurement between two regions.
type Result struct {
	Source    string    `json:"source"`
	Target    string    `json:"target"`
	RTTMs     float64   `json:"rtt_ms"`
	MinMs     float64   `json:"min_ms"`
	MaxMs     float64   `json:"max_ms"`
	P50Ms     float64   `json:"p50_ms"`
	Samples   int       `json:"samples"`
	Timestamp time.Time `json:"timestamp"`
}

// Matrix stores the latest and historical latency measurements for all region pairs.
type Matrix struct {
	mu      sync.RWMutex
	latest  map[string]map[string]*Result // source -> target -> result
	history map[string][]*Result          // "source:target" -> rolling history
	maxHist int
}

// NewMatrix creates a new latency matrix with the given history depth.
func NewMatrix(maxHistory int) *Matrix {
	return &Matrix{
		latest:  make(map[string]map[string]*Result),
		history: make(map[string][]*Result),
		maxHist: maxHistory,
	}
}

// Update records a new measurement.
func (m *Matrix) Update(r *Result) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.latest[r.Source] == nil {
		m.latest[r.Source] = make(map[string]*Result)
	}
	m.latest[r.Source][r.Target] = r

	key := r.Source + ":" + r.Target
	hist := m.history[key]
	hist = append(hist, r)
	if len(hist) > m.maxHist {
		hist = hist[len(hist)-m.maxHist:]
	}
	m.history[key] = hist
}

// Get returns the latest measurement for a pair, or nil.
func (m *Matrix) Get(source, target string) *Result {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if row, ok := m.latest[source]; ok {
		return row[target]
	}
	return nil
}

// GetRow returns all latest measurements from a source region.
func (m *Matrix) GetRow(source string) []*Result {
	m.mu.RLock()
	defer m.mu.RUnlock()
	row := m.latest[source]
	results := make([]*Result, 0, len(row))
	for _, r := range row {
		results = append(results, r)
	}
	return results
}

// GetAll returns the full latest matrix as a flat slice.
func (m *Matrix) GetAll() []*Result {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var results []*Result
	for _, row := range m.latest {
		for _, r := range row {
			results = append(results, r)
		}
	}
	return results
}

// GetHistory returns the measurement history for a specific pair.
func (m *Matrix) GetHistory(source, target string) []*Result {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := source + ":" + target
	hist := m.history[key]
	out := make([]*Result, len(hist))
	copy(out, hist)
	return out
}

// Sources returns all source regions that have reported data.
func (m *Matrix) Sources() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sources := make([]string, 0, len(m.latest))
	for s := range m.latest {
		sources = append(sources, s)
	}
	return sources
}

// PairCount returns the number of unique measured pairs.
func (m *Matrix) PairCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, row := range m.latest {
		count += len(row)
	}
	return count
}
