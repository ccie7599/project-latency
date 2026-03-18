package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bapley/project-latency/internal/latency"
	"github.com/bapley/project-latency/internal/regions"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

//go:embed static
var staticFiles embed.FS

// Prometheus metrics
var (
	routeRTT = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nats_cluster_route_rtt_seconds",
			Help: "RTT between NATS cluster nodes in seconds",
		},
		[]string{"source", "target"},
	)
	routeCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nats_cluster_node_routes",
			Help: "Number of cluster routes per node",
		},
		[]string{"node"},
	)
	scrapeErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "latency_scrape_errors_total",
			Help: "Total number of routez scrape errors",
		},
	)
)

func init() {
	prometheus.MustRegister(routeRTT, routeCount, scrapeErrors)
}

type Hub struct {
	region     string
	natsServer *server.Server
	promAPI    promv1.API
	wsClients  sync.Map
	authToken  string
	httpClient *http.Client
}

// routezResponse mirrors the NATS /routez JSON response.
type routezResponse struct {
	ServerID   string      `json:"server_id"`
	ServerName string      `json:"server_name"`
	NumRoutes  int         `json:"num_routes"`
	Routes     []routeInfo `json:"routes"`
}

type routeInfo struct {
	RemoteID   string `json:"remote_id"`
	RemoteName string `json:"remote_name"`
	IP         string `json:"ip"`
	Port       int    `json:"port"`
	RTT        string `json:"rtt"`
}

func main() {
	hubRegion := os.Getenv("REGION")
	if hubRegion == "" {
		hubRegion = "us-ord"
	}
	seedRoutes := os.Getenv("NATS_SEED_ROUTES") // empty for hub (it IS the seed)
	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":443"
	}
	tlsCert := os.Getenv("TLS_CERT")
	tlsKey := os.Getenv("TLS_KEY")
	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":2112"
	}
	promURL := os.Getenv("PROMETHEUS_URL")
	if promURL == "" {
		promURL = "http://localhost:9090"
	}
	authToken := os.Getenv("AUTH_TOKEN")

	clusterPort := 6222
	monitorPort := 8222
	clientPort := 4222

	// Parse seed routes (hub may have none if it's the initial seed)
	var routes []*url.URL
	if seedRoutes != "" {
		routes = server.RoutesFromStr(seedRoutes)
	}

	// Start embedded NATS server as cluster member
	opts := &server.Options{
		ServerName: hubRegion,
		Host:       "0.0.0.0",
		Port:       clientPort,
		HTTPPort:   monitorPort,
		Cluster: server.ClusterOpts{
			Name: "latency-mesh",
			Host: "0.0.0.0",
			Port: clusterPort,
		},
		Routes: routes,
		NoSigs: true,
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		log.Fatalf("nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(30 * time.Second) {
		log.Fatal("nats server not ready")
	}
	log.Printf("NATS cluster member started: client=%d cluster=%d monitor=%d", clientPort, clusterPort, monitorPort)

	// Prometheus API client (for PromQL queries)
	promClient, err := promapi.NewClient(promapi.Config{Address: promURL})
	if err != nil {
		log.Fatalf("prometheus client: %v", err)
	}

	hub := &Hub{
		region:     hubRegion,
		natsServer: ns,
		promAPI:    promv1.NewAPI(promClient),
		authToken:  authToken,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	// Start Prometheus metrics endpoint
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Printf("Metrics listening on %s", metricsAddr)
		http.ListenAndServe(metricsAddr, metricsMux)
	}()

	// Start routez scraper
	go hub.scrapeLoop()

	// HTTP API + frontend server
	mux := http.NewServeMux()

	// Protected endpoints — require ?auth=TOKEN
	mux.HandleFunc("/api/v1/matrix", hub.requireAuth(hub.handleMatrix))
	mux.HandleFunc("/api/v1/pair/", hub.requireAuth(hub.handlePair))
	mux.HandleFunc("/api/v1/regions", hub.requireAuth(hub.handleRegions))
	mux.HandleFunc("/api/v1/nearest", hub.requireAuth(hub.handleNearest))
	mux.HandleFunc("/api/v1/az-pairs", hub.requireAuth(hub.handleAZPairs))
	mux.HandleFunc("/api/v1/health", hub.requireAuth(hub.handleHealthAPI))
	mux.HandleFunc("/api/v1/cluster", hub.requireAuth(hub.handleCluster))
	mux.HandleFunc("/ws/live", hub.requireAuth(hub.handleWebSocket))

	// Unauthenticated — /ping for client latency measurement
	mux.HandleFunc("/ping", hub.handlePing)

	// Serve index.html with token injected, static assets as-is
	staticFS, _ := fs.Sub(staticFiles, "static")
	fileServer := http.FileServer(http.FS(staticFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			hub.serveIndexWithToken(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	httpServer := &http.Server{
		Addr:         listenAddr,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	go func() {
		if tlsCert != "" && tlsKey != "" {
			log.Printf("HTTPS listening on %s", listenAddr)
			if err := httpServer.ListenAndServeTLS(tlsCert, tlsKey); err != nil && err != http.ErrServerClosed {
				log.Fatalf("https: %v", err)
			}
		} else {
			log.Printf("HTTP listening on %s (no TLS)", listenAddr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("http: %v", err)
			}
		}
	}()

	// Wait for shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Printf("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
	ns.Shutdown()
}

// ==========================================================================
// Routez scraper — collects RTT from all NATS cluster members
// ==========================================================================

func (h *Hub) scrapeLoop() {
	// Wait for cluster to form
	time.Sleep(15 * time.Second)
	h.scrapeAllNodes()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		h.scrapeAllNodes()
	}
}

func (h *Hub) scrapeAllNodes() {
	// Step 1: Scrape our own monitoring endpoint to discover cluster members
	selfRoutez, err := h.fetchRoutez(fmt.Sprintf("http://127.0.0.1:%d", 8222))
	if err != nil {
		log.Printf("scrape self: %v", err)
		scrapeErrors.Inc()
		return
	}

	// Record hub's own routes
	routeCount.WithLabelValues(h.region).Set(float64(selfRoutez.NumRoutes))
	for _, route := range selfRoutez.Routes {
		if route.RemoteName == "" || route.RTT == "" {
			continue
		}
		rttSec := parseRTT(route.RTT)
		if rttSec > 0 {
			routeRTT.WithLabelValues(h.region, route.RemoteName).Set(rttSec)
		}
	}

	// Step 2: Scrape each cluster member's /routez for the full mesh
	var wg sync.WaitGroup
	for _, route := range selfRoutez.Routes {
		if route.RemoteName == "" || route.IP == "" {
			continue
		}
		wg.Add(1)
		go func(name, ip string) {
			defer wg.Done()
			memberRoutez, err := h.fetchRoutez(fmt.Sprintf("http://%s:%d", ip, 8222))
			if err != nil {
				scrapeErrors.Inc()
				return
			}
			routeCount.WithLabelValues(name).Set(float64(memberRoutez.NumRoutes))
			for _, mr := range memberRoutez.Routes {
				if mr.RemoteName == "" || mr.RTT == "" {
					continue
				}
				rttSec := parseRTT(mr.RTT)
				if rttSec > 0 {
					routeRTT.WithLabelValues(name, mr.RemoteName).Set(rttSec)
				}
			}
		}(route.RemoteName, route.IP)
	}
	wg.Wait()

	// Broadcast update to WebSocket clients
	h.broadcastLatest()
}

func (h *Hub) fetchRoutez(baseURL string) (*routezResponse, error) {
	resp, err := h.httpClient.Get(baseURL + "/routez")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rz routezResponse
	if err := json.NewDecoder(resp.Body).Decode(&rz); err != nil {
		return nil, err
	}
	return &rz, nil
}

// parseRTT converts a NATS RTT string (e.g., "15.234ms") to seconds.
func parseRTT(s string) float64 {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d.Seconds()
}

// ==========================================================================
// PromQL query helpers
// ==========================================================================

func (h *Hub) queryMatrix(ctx context.Context) ([]latency.Result, error) {
	result, _, err := h.promAPI.Query(ctx, "nats_cluster_route_rtt_seconds * 1000", time.Now())
	if err != nil {
		return nil, err
	}

	vec, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	var results []latency.Result
	for _, sample := range vec {
		source := string(sample.Metric["source"])
		target := string(sample.Metric["target"])
		if source == "" || target == "" {
			continue
		}
		results = append(results, latency.Result{
			Source:    source,
			Target:    target,
			RTTMs:     math.Round(float64(sample.Value)*100) / 100,
			Timestamp: sample.Timestamp.Time(),
		})
	}
	return results, nil
}

func (h *Hub) queryPairHistory(ctx context.Context, source, target string, duration time.Duration) ([]latency.Result, error) {
	query := fmt.Sprintf(`nats_cluster_route_rtt_seconds{source="%s",target="%s"} * 1000`, source, target)
	end := time.Now()
	start := end.Add(-duration)

	result, _, err := h.promAPI.QueryRange(ctx, query, promv1.Range{
		Start: start,
		End:   end,
		Step:  30 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	matrix, ok := result.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	var results []latency.Result
	for _, stream := range matrix {
		for _, sample := range stream.Values {
			results = append(results, latency.Result{
				Source:    source,
				Target:    target,
				RTTMs:     math.Round(float64(sample.Value)*100) / 100,
				Timestamp: sample.Timestamp.Time(),
			})
		}
	}
	return results, nil
}

func (h *Hub) querySourceRow(ctx context.Context, source string) ([]latency.Result, error) {
	query := fmt.Sprintf(`nats_cluster_route_rtt_seconds{source="%s"} * 1000`, source)
	result, _, err := h.promAPI.Query(ctx, query, time.Now())
	if err != nil {
		return nil, err
	}

	vec, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	var results []latency.Result
	for _, sample := range vec {
		target := string(sample.Metric["target"])
		if target == "" {
			continue
		}
		results = append(results, latency.Result{
			Source:    source,
			Target:    target,
			RTTMs:     math.Round(float64(sample.Value)*100) / 100,
			Timestamp: sample.Timestamp.Time(),
		})
	}
	return results, nil
}

// ==========================================================================
// API handlers
// ==========================================================================

func (h *Hub) handleMatrix(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")

	var results []latency.Result
	var err error
	if from != "" {
		results, err = h.querySourceRow(r.Context(), from)
	} else {
		results, err = h.queryMatrix(r.Context())
	}
	if err != nil {
		log.Printf("matrix query: %v", err)
		writeJSON(w, map[string]interface{}{"results": []interface{}{}, "count": 0, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]interface{}{
		"results": results,
		"count":   len(results),
		"ts":      time.Now().Unix(),
	})
}

func (h *Hub) handlePair(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/pair/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "expected /api/v1/pair/{from}/{to}", http.StatusBadRequest)
		return
	}
	from, to := parts[0], parts[1]

	// Duration from query param, default 1h
	dur := 1 * time.Hour
	if d := r.URL.Query().Get("duration"); d != "" {
		if parsed, err := time.ParseDuration(d); err == nil {
			dur = parsed
		}
	}

	history, err := h.queryPairHistory(r.Context(), from, to, dur)
	if err != nil {
		log.Printf("pair query: %v", err)
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}

	var latest *latency.Result
	if len(history) > 0 {
		latest = &history[len(history)-1]
	}

	writeJSON(w, map[string]interface{}{
		"latest":  latest,
		"history": history,
	})
}

func (h *Hub) handleRegions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, regions.All)
}

func (h *Hub) handleNearest(w http.ResponseWriter, r *http.Request) {
	lat := r.URL.Query().Get("lat")
	lon := r.URL.Query().Get("lon")

	if lat != "" && lon != "" {
		var clat, clon float64
		fmt.Sscanf(lat, "%f", &clat)
		fmt.Sscanf(lon, "%f", &clon)

		type regionDist struct {
			regions.Region
			DistanceKm float64 `json:"distance_km"`
		}

		var results []regionDist
		for _, reg := range regions.All {
			dist := regions.HaversineKm(clat, clon, reg.Lat, reg.Lon)
			results = append(results, regionDist{Region: reg, DistanceKm: dist})
		}
		sort.Slice(results, func(i, j int) bool {
			return results[i].DistanceKm < results[j].DistanceKm
		})
		if len(results) > 10 {
			results = results[:10]
		}
		writeJSON(w, map[string]interface{}{
			"client_lat": clat,
			"client_lon": clon,
			"nearest":    results,
		})
		return
	}

	http.Error(w, "provide lat/lon query params", http.StatusBadRequest)
}

func (h *Hub) handleAZPairs(w http.ResponseWriter, r *http.Request) {
	primary := r.URL.Query().Get("primary")
	if primary == "" {
		http.Error(w, "primary query param required", http.StatusBadRequest)
		return
	}

	results, err := h.queryMatrix(r.Context())
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}

	pairs := latency.FindAZPairs(primary, results)
	writeJSON(w, map[string]interface{}{
		"primary": primary,
		"pairs":   pairs,
	})
}

func (h *Hub) handleHealthAPI(w http.ResponseWriter, r *http.Request) {
	numRoutes := h.natsServer.NumRoutes()
	writeJSON(w, map[string]interface{}{
		"status":     "ok",
		"region":     h.region,
		"num_routes": numRoutes,
		"ts":         time.Now().Unix(),
	})
}

func (h *Hub) handleCluster(w http.ResponseWriter, r *http.Request) {
	rz, err := h.fetchRoutez("http://127.0.0.1:8222")
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}

	type member struct {
		Name      string `json:"name"`
		IP        string `json:"ip"`
		RTTMs     string `json:"rtt"`
	}
	var members []member
	for _, route := range rz.Routes {
		members = append(members, member{
			Name:  route.RemoteName,
			IP:    route.IP,
			RTTMs: route.RTT,
		})
	}
	writeJSON(w, map[string]interface{}{
		"hub":     h.region,
		"members": members,
		"count":   len(members),
	})
}

func (h *Hub) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"region": h.region,
		"role":   "hub",
		"ts":     time.Now().UnixMilli(),
	})
}

// ==========================================================================
// WebSocket
// ==========================================================================

func (h *Hub) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	h.wsClients.Store(conn, ctx)
	defer func() {
		h.wsClients.Delete(conn)
		cancel()
		conn.Close(websocket.StatusNormalClosure, "")
	}()

	// Send current data on connect
	results, err := h.queryMatrix(ctx)
	if err == nil {
		wsjson.Write(ctx, conn, map[string]interface{}{
			"type":    "snapshot",
			"results": results,
		})
	}

	// Read loop (keep-alive)
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			return
		}
	}
}

func (h *Hub) broadcastLatest() {
	ctx := context.Background()
	results, err := h.queryMatrix(ctx)
	if err != nil {
		return
	}

	msg := map[string]interface{}{
		"type":    "update",
		"results": results,
	}

	h.wsClients.Range(func(key, value interface{}) bool {
		conn := key.(*websocket.Conn)
		wsCtx := value.(context.Context)
		wsjson.Write(wsCtx, conn, msg)
		return true
	})
}

// ==========================================================================
// Auth middleware + token injection
// ==========================================================================

func (h *Hub) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.authToken == "" {
			// No token configured — allow all
			next(w, r)
			return
		}
		if r.URL.Query().Get("auth") != h.authToken {
			http.Error(w, `{"error":"invalid or missing token"}`, http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (h *Hub) serveIndexWithToken(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	// Remove the server-side token placeholder — client validates via API
	html := strings.Replace(string(data), "__AUTH_TOKEN__", "", -1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(html))
}

// ==========================================================================
// Helpers
// ==========================================================================

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
