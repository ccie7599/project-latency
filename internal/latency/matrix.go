package latency

import "time"

// Result is a single latency measurement between two regions.
// Used as a transfer type between the scraper, Prometheus, and the API layer.
type Result struct {
	Source    string    `json:"source"`
	Target    string    `json:"target"`
	RTTMs     float64   `json:"rtt_ms"`
	Timestamp time.Time `json:"timestamp"`
}
