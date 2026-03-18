package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
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
	"github.com/nats-io/nats.go"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

//go:embed static
var staticFiles embed.FS

type Hub struct {
	natsServer *server.Server
	nc         *nats.Conn
	matrix     *latency.Matrix
	wsClients  sync.Map // map[*websocket.Conn]context.CancelFunc
	authToken  string
}

func main() {
	authToken := os.Getenv("AUTH_TOKEN")
	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":8443"
	}
	natsPort := 4222
	leafPort := 7422

	hub := &Hub{
		matrix:    latency.NewMatrix(96), // 24h at 15-min intervals
		authToken: authToken,
	}

	// Start embedded NATS server
	opts := &server.Options{
		Host:       "0.0.0.0",
		Port:       natsPort,
		MaxPayload: 64 * 1024,
		LeafNode: server.LeafNodeOpts{
			Host: "0.0.0.0",
			Port: leafPort,
		},
		NoSigs: true,
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		log.Fatalf("nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		log.Fatal("nats server not ready")
	}
	hub.natsServer = ns
	log.Printf("NATS server started: client=%d leaf=%d", natsPort, leafPort)

	// Connect local NATS client
	nc, err := nats.Connect(fmt.Sprintf("nats://127.0.0.1:%d", natsPort),
		nats.Name("hub-aggregator"),
	)
	if err != nil {
		log.Fatalf("nats connect: %v", err)
	}
	hub.nc = nc

	// Subscribe to latency results from all agents
	nc.Subscribe("latency.results.>", func(msg *nats.Msg) {
		var r latency.Result
		if err := json.Unmarshal(msg.Data, &r); err != nil {
			log.Printf("bad result: %v", err)
			return
		}
		hub.matrix.Update(&r)
		hub.broadcastToWS(&r)
	})

	// Subscribe to health reports
	nc.Subscribe("latency.health.>", func(msg *nats.Msg) {
		// Log health reports
		parts := strings.Split(msg.Subject, ".")
		if len(parts) >= 3 {
			log.Printf("health: %s alive", parts[2])
		}
	})

	// Schedule periodic sweeps
	go hub.sweepScheduler()

	// HTTP server
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/v1/matrix", hub.handleMatrix)
	mux.HandleFunc("/api/v1/pair/", hub.handlePair)
	mux.HandleFunc("/api/v1/regions", hub.handleRegions)
	mux.HandleFunc("/api/v1/nearest", hub.handleNearest)
	mux.HandleFunc("/api/v1/az-pairs", hub.handleAZPairs)
	mux.HandleFunc("/api/v1/health", hub.handleHealthAPI)
	mux.HandleFunc("/api/v1/trigger-sweep", hub.handleTriggerSweep)
	mux.HandleFunc("/ws/live", hub.handleWebSocket)
	mux.HandleFunc("/ping", hub.handlePing)

	// Static frontend files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	httpServer := &http.Server{
		Addr:         listenAddr,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("HTTP listening on %s", listenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
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
	nc.Drain()
	ns.Shutdown()
}

// sweepScheduler triggers a full measurement sweep every 15 minutes.
func (h *Hub) sweepScheduler() {
	// Initial sweep after 10 seconds (give agents time to connect)
	time.Sleep(10 * time.Second)
	h.triggerSweep()

	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		h.triggerSweep()
	}
}

func (h *Hub) triggerSweep() {
	regionIDs := regions.IDs()
	cmd, _ := json.Marshal(map[string]interface{}{
		"regions": regionIDs,
	})
	if err := h.nc.Publish("latency.control.schedule", cmd); err != nil {
		log.Printf("trigger sweep: %v", err)
	} else {
		log.Printf("triggered sweep for %d regions", len(regionIDs))
	}
}

// broadcastToWS sends a latency result to all connected WebSocket clients.
func (h *Hub) broadcastToWS(r *latency.Result) {
	h.wsClients.Range(func(key, value interface{}) bool {
		conn := key.(*websocket.Conn)
		ctx := value.(context.Context)
		go func() {
			wsjson.Write(ctx, conn, r)
		}()
		return true
	})
}

// API handlers

func (h *Hub) handleMatrix(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	var results []*latency.Result
	if from != "" {
		results = h.matrix.GetRow(from)
	} else {
		results = h.matrix.GetAll()
	}
	writeJSON(w, map[string]interface{}{
		"results": results,
		"count":   len(results),
		"ts":      time.Now().Unix(),
	})
}

func (h *Hub) handlePair(w http.ResponseWriter, r *http.Request) {
	// /api/v1/pair/{from}/{to}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/pair/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "expected /api/v1/pair/{from}/{to}", http.StatusBadRequest)
		return
	}
	from, to := parts[0], parts[1]

	latest := h.matrix.Get(from, to)
	history := h.matrix.GetHistory(from, to)

	writeJSON(w, map[string]interface{}{
		"latest":  latest,
		"history": history,
	})
}

func (h *Hub) handleRegions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, regions.All)
}

func (h *Hub) handleNearest(w http.ResponseWriter, r *http.Request) {
	// Use query param ip, or infer from request
	ip := r.URL.Query().Get("ip")
	lat := r.URL.Query().Get("lat")
	lon := r.URL.Query().Get("lon")

	if lat != "" && lon != "" {
		// Direct lat/lon provided
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

	if ip != "" {
		// GeoIP lookup would go here — for now return error
		writeJSON(w, map[string]interface{}{
			"error": "GeoIP lookup not yet implemented, use lat/lon params",
			"ip":    ip,
		})
		return
	}

	http.Error(w, "provide lat/lon or ip query params", http.StatusBadRequest)
}

func (h *Hub) handleAZPairs(w http.ResponseWriter, r *http.Request) {
	primary := r.URL.Query().Get("primary")
	if primary == "" {
		http.Error(w, "primary query param required", http.StatusBadRequest)
		return
	}

	minDist := 200.0 // km
	if d := r.URL.Query().Get("min_distance_km"); d != "" {
		fmt.Sscanf(d, "%f", &minDist)
	}

	pairs := latency.FindAZPairs(primary, h.matrix, minDist)
	writeJSON(w, map[string]interface{}{
		"primary": primary,
		"pairs":   pairs,
	})
}

func (h *Hub) handleHealthAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"status":         "ok",
		"nats_connected": h.nc.IsConnected(),
		"pairs_measured": h.matrix.PairCount(),
		"sources":        h.matrix.Sources(),
		"ts":             time.Now().Unix(),
	})
}

func (h *Hub) handleTriggerSweep(w http.ResponseWriter, r *http.Request) {
	if h.authToken != "" && r.URL.Query().Get("auth") != h.authToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.triggerSweep()
	writeJSON(w, map[string]interface{}{"status": "sweep triggered"})
}

func (h *Hub) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"region": "us-ord",
		"role":   "hub",
		"ts":     time.Now().UnixMilli(),
	})
}

func (h *Hub) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		log.Printf("ws accept: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	h.wsClients.Store(conn, ctx)
	defer func() {
		h.wsClients.Delete(conn)
		cancel()
		conn.Close(websocket.StatusNormalClosure, "")
	}()

	// Send current matrix snapshot on connect
	snapshot := h.matrix.GetAll()
	wsjson.Write(ctx, conn, map[string]interface{}{
		"type":    "snapshot",
		"results": snapshot,
	})

	// Keep alive — read loop (client can send commands later)
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			return
		}
	}
}

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
