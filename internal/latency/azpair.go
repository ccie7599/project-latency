package latency

import (
	"sort"

	"github.com/bapley/project-latency/internal/regions"
)

// AZCategory classifies a region pair's suitability for different replication strategies.
type AZCategory string

const (
	AZSyncCapable AZCategory = "sync-capable"  // <10ms RTT, viable for synchronous replication
	AZAsyncLowRPO AZCategory = "async-low-rpo" // 10-50ms RTT, low RPO async replication
	AZDROnly      AZCategory = "dr-only"       // >50ms RTT, disaster recovery only
)

// AZPair represents a recommended availability zone pair.
type AZPair struct {
	Primary     string     `json:"primary"`
	Secondary   string     `json:"secondary"`
	RTTMs       float64    `json:"rtt_ms"`
	DistanceKm  float64    `json:"distance_km"`
	Category    AZCategory `json:"category"`
	SameCountry bool       `json:"same_country"`
	CoLocated   bool       `json:"co_located"` // same city, not suitable as AZ pair
	Rationale   string     `json:"rationale"`
}

// ClassifyAZPair determines the AZ category for a given latency and distance.
func ClassifyAZPair(rttMs, distanceKm float64, sameCountry bool) (AZCategory, string) {
	switch {
	case rttMs < 10:
		if distanceKm < 50 {
			return AZSyncCapable, "Co-located in same metro — sync replication viable but limited fault isolation"
		}
		return AZSyncCapable, "Low latency enables synchronous replication with geographic separation"
	case rttMs < 50:
		if sameCountry {
			return AZAsyncLowRPO, "Same country, moderate latency — async replication with low RPO"
		}
		return AZAsyncLowRPO, "Cross-border, moderate latency — async replication with low RPO, check data sovereignty"
	default:
		return AZDROnly, "High latency — suitable for disaster recovery with higher RPO tolerance"
	}
}

// FindAZPairs returns recommended AZ pairs for a given primary region, sorted by RTT.
func FindAZPairs(primary string, matrix *Matrix, minDistanceKm float64) []AZPair {
	regionMap := regions.ByID()
	prim, ok := regionMap[primary]
	if !ok {
		return nil
	}

	var pairs []AZPair
	for _, r := range regions.All {
		if r.ID == primary {
			continue
		}

		dist := regions.HaversineKm(prim.Lat, prim.Lon, r.Lat, r.Lon)
		sameCountry := prim.Country == r.Country
		coLocated := dist < 50

		// Get bidirectional latency, average if both exist
		rttMs := 0.0
		count := 0
		if fwd := matrix.Get(primary, r.ID); fwd != nil {
			rttMs += fwd.P50Ms
			count++
		}
		if rev := matrix.Get(r.ID, primary); rev != nil {
			rttMs += rev.P50Ms
			count++
		}
		if count == 0 {
			continue // no measurement data
		}
		rttMs /= float64(count)

		cat, rationale := ClassifyAZPair(rttMs, dist, sameCountry)

		pairs = append(pairs, AZPair{
			Primary:     primary,
			Secondary:   r.ID,
			RTTMs:       rttMs,
			DistanceKm:  dist,
			Category:    cat,
			SameCountry: sameCountry,
			CoLocated:   coLocated,
			Rationale:   rationale,
		})
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].RTTMs < pairs[j].RTTMs
	})

	return pairs
}
