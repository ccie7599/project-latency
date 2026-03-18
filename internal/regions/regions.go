package regions

import "math"

type Region struct {
	ID      string  `json:"id"`
	Label   string  `json:"label"`
	Country string  `json:"country"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Short   string  `json:"short"` // short name for DNS/display
}

// All contains every current Linode region with geographic coordinates.
var All = []Region{
	// North America
	{ID: "us-ord", Label: "Chicago, IL", Country: "us", Lat: 41.8781, Lon: -87.6298, Short: "ord"},
	{ID: "us-east", Label: "Newark, NJ", Country: "us", Lat: 40.7357, Lon: -74.1724, Short: "ewr"},
	{ID: "us-central", Label: "Dallas, TX", Country: "us", Lat: 32.7767, Lon: -96.7970, Short: "dfw"},
	{ID: "us-west", Label: "Fremont, CA", Country: "us", Lat: 37.5485, Lon: -121.9886, Short: "fmt"},
	{ID: "us-southeast", Label: "Atlanta, GA", Country: "us", Lat: 33.7490, Lon: -84.3880, Short: "atl"},
	{ID: "us-lax", Label: "Los Angeles, CA", Country: "us", Lat: 34.0522, Lon: -118.2437, Short: "lax"},
	{ID: "us-mia", Label: "Miami, FL", Country: "us", Lat: 25.7617, Lon: -80.1918, Short: "mia"},
	{ID: "us-sea", Label: "Seattle, WA", Country: "us", Lat: 47.6062, Lon: -122.3321, Short: "sea"},
	{ID: "us-iad", Label: "Washington, DC", Country: "us", Lat: 38.9072, Lon: -77.0369, Short: "iad"},
	{ID: "us-iad-2", Label: "Washington 2, DC", Country: "us", Lat: 38.9072, Lon: -77.0369, Short: "iad2"},
	{ID: "us-den-1", Label: "Denver, CO", Country: "us", Lat: 39.7392, Lon: -104.9903, Short: "den"},
	{ID: "us-hou-1", Label: "Houston, TX", Country: "us", Lat: 29.7604, Lon: -95.3698, Short: "hou"},
	{ID: "ca-central", Label: "Toronto, CA", Country: "ca", Lat: 43.6532, Lon: -79.3832, Short: "yyz"},

	// South America
	{ID: "br-gru", Label: "Sao Paulo, BR", Country: "br", Lat: -23.5505, Lon: -46.6333, Short: "gru"},
	{ID: "co-bog-1", Label: "Bogotá, CO", Country: "co", Lat: 4.7110, Lon: -74.0721, Short: "bog1"},
	{ID: "co-bog-2", Label: "Bogotá 2, CO", Country: "co", Lat: 4.7110, Lon: -74.0721, Short: "bog2"},
	{ID: "mx-qro-1", Label: "Querétaro, MX", Country: "mx", Lat: 20.5888, Lon: -100.3899, Short: "qro"},
	{ID: "cl-scl-1", Label: "Santiago, CL", Country: "cl", Lat: -33.4489, Lon: -70.6693, Short: "scl"},

	// Europe
	{ID: "eu-west", Label: "London, UK", Country: "gb", Lat: 51.5074, Lon: -0.1278, Short: "lon"},
	{ID: "gb-lon", Label: "London 2, UK", Country: "gb", Lat: 51.5074, Lon: -0.1278, Short: "lon2"},
	{ID: "eu-central", Label: "Frankfurt, DE", Country: "de", Lat: 50.1109, Lon: 8.6821, Short: "fra"},
	{ID: "de-fra-2", Label: "Frankfurt 2, DE", Country: "de", Lat: 50.1109, Lon: 8.6821, Short: "fra2"},
	{ID: "fr-par", Label: "Paris, FR", Country: "fr", Lat: 48.8566, Lon: 2.3522, Short: "par"},
	{ID: "fr-par-2", Label: "Paris 2, FR", Country: "fr", Lat: 48.8566, Lon: 2.3522, Short: "par2"},
	{ID: "nl-ams", Label: "Amsterdam, NL", Country: "nl", Lat: 52.3676, Lon: 4.9041, Short: "ams"},
	{ID: "se-sto", Label: "Stockholm, SE", Country: "se", Lat: 59.3293, Lon: 18.0686, Short: "sto"},
	{ID: "es-mad", Label: "Madrid, ES", Country: "es", Lat: 40.4168, Lon: -3.7038, Short: "mad"},
	{ID: "it-mil", Label: "Milan, IT", Country: "it", Lat: 45.4642, Lon: 9.1900, Short: "mil"},
	{ID: "de-ber-1", Label: "Berlin, DE", Country: "de", Lat: 52.5200, Lon: 13.4050, Short: "ber"},
	{ID: "de-ham-1", Label: "Hamburg, DE", Country: "de", Lat: 53.5511, Lon: 9.9937, Short: "ham"},
	{ID: "no-osl-1", Label: "Oslo, NO", Country: "no", Lat: 59.9139, Lon: 10.7522, Short: "osl"},
	{ID: "fr-mrs-2", Label: "Marseille 2, FR", Country: "fr", Lat: 43.2965, Lon: 5.3698, Short: "mrs"},

	// Asia Pacific
	{ID: "ap-south", Label: "Singapore, SG", Country: "sg", Lat: 1.3521, Lon: 103.8198, Short: "sin"},
	{ID: "sg-sin-2", Label: "Singapore 2, SG", Country: "sg", Lat: 1.3521, Lon: 103.8198, Short: "sin2"},
	{ID: "ap-northeast", Label: "Tokyo 2, JP", Country: "jp", Lat: 35.6762, Lon: 139.6503, Short: "tyo2"},
	{ID: "jp-tyo-3", Label: "Tokyo 3, JP", Country: "jp", Lat: 35.6762, Lon: 139.6503, Short: "tyo3"},
	{ID: "jp-osa", Label: "Osaka, JP", Country: "jp", Lat: 34.6937, Lon: 135.5023, Short: "osa"},
	{ID: "ap-west", Label: "Mumbai, IN", Country: "in", Lat: 19.0760, Lon: 72.8777, Short: "bom"},
	{ID: "in-bom-2", Label: "Mumbai 2, IN", Country: "in", Lat: 19.0760, Lon: 72.8777, Short: "bom2"},
	{ID: "in-maa", Label: "Chennai, IN", Country: "in", Lat: 13.0827, Lon: 80.2707, Short: "maa"},
	{ID: "id-cgk", Label: "Jakarta, ID", Country: "id", Lat: -6.2088, Lon: 106.8456, Short: "cgk"},
	{ID: "my-kul-1", Label: "Kuala Lumpur, MY", Country: "my", Lat: 3.1390, Lon: 101.6869, Short: "kul"},

	// Oceania
	{ID: "ap-southeast", Label: "Sydney, AU", Country: "au", Lat: -33.8688, Lon: 151.2093, Short: "syd"},
	{ID: "au-mel", Label: "Melbourne, AU", Country: "au", Lat: -37.8136, Lon: 144.9631, Short: "mel"},
	{ID: "nz-akl-1", Label: "Auckland, NZ", Country: "nz", Lat: -36.8485, Lon: 174.7633, Short: "akl"},

	// Africa
	{ID: "za-jnb-1", Label: "Johannesburg, ZA", Country: "za", Lat: -26.2041, Lon: 28.0473, Short: "jnb"},
}

// ByID returns a map keyed by region ID for fast lookup.
func ByID() map[string]Region {
	m := make(map[string]Region, len(All))
	for _, r := range All {
		m[r.ID] = r
	}
	return m
}

// IDs returns all region IDs.
func IDs() []string {
	ids := make([]string, len(All))
	for i, r := range All {
		ids[i] = r.ID
	}
	return ids
}

// HaversineKm returns the great-circle distance in kilometers between two lat/lon points.
func HaversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0 // Earth radius in km
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}
