package latency

import (
	"sort"

	"github.com/bapley/project-latency/internal/regions"
)

type AZCategory string

const (
	AZSyncCapable AZCategory = "sync-capable"  // <10ms RTT
	AZAsyncLowRPO AZCategory = "async-low-rpo" // 10-50ms RTT
	AZDROnly      AZCategory = "dr-only"       // >50ms RTT
)

type AZPair struct {
	Primary     string     `json:"primary"`
	Secondary   string     `json:"secondary"`
	RTTMs       float64    `json:"rtt_ms"`
	DistanceKm  float64    `json:"distance_km"`
	Category    AZCategory `json:"category"`
	SameCountry bool       `json:"same_country"`
	CoLocated   bool       `json:"co_located"`
	Rationale   string     `json:"rationale"`
}

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

// FindAZPairs computes AZ pair recommendations from a flat slice of results.
// The results slice should contain RTT measurements where source == primary.
func FindAZPairs(primary string, results []Result) []AZPair {
	regionMap := regions.ByID()
	prim, ok := regionMap[primary]
	if !ok {
		return nil
	}

	// Build RTT lookup from results
	rttByTarget := make(map[string]float64)
	for _, r := range results {
		if r.Source == primary {
			rttByTarget[r.Target] = r.RTTMs
		}
	}

	var pairs []AZPair
	for target, rttMs := range rttByTarget {
		tgt, ok := regionMap[target]
		if !ok {
			continue
		}

		dist := regions.HaversineKm(prim.Lat, prim.Lon, tgt.Lat, tgt.Lon)
		sameCountry := prim.Country == tgt.Country
		coLocated := dist < 50

		cat, rationale := ClassifyAZPair(rttMs, dist, sameCountry)

		pairs = append(pairs, AZPair{
			Primary:     primary,
			Secondary:   target,
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
