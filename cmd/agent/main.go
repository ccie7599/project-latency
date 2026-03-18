package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
)

type Config struct {
	Region      string
	NATSCoreURL string // nats://user:pass@hub:7422
	ListenAddr  string
}

func loadConfig() Config {
	region := os.Getenv("REGION")
	if region == "" {
		log.Fatal("REGION environment variable required")
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}
	listen := os.Getenv("LISTEN_ADDR")
	if listen == "" {
		listen = ":8080"
	}
	return Config{
		Region:      region,
		NATSCoreURL: natsURL,
		ListenAddr:  listen,
	}
}

type Agent struct {
	cfg  Config
	nc   *nats.Conn
	done chan struct{}
}

func main() {
	cfg := loadConfig()
	log.Printf("agent starting: region=%s nats=%s listen=%s", cfg.Region, cfg.NATSCoreURL, cfg.ListenAddr)

	agent := &Agent{
		cfg:  cfg,
		done: make(chan struct{}),
	}

	// Connect to NATS hub
	nc, err := nats.Connect(cfg.NATSCoreURL,
		nats.Name(fmt.Sprintf("probe-%s", cfg.Region)),
		nats.ReconnectWait(2*time.Second),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Printf("nats disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			log.Printf("nats reconnected")
		}),
	)
	if err != nil {
		log.Fatalf("nats connect: %v", err)
	}
	agent.nc = nc
	log.Printf("connected to NATS hub")

	// Subscribe to probe requests for this region
	sub, err := nc.Subscribe(fmt.Sprintf("latency.probe.request.%s", cfg.Region), agent.handleProbeRequest)
	if err != nil {
		log.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Subscribe to schedule commands from hub
	scheduleSub, err := nc.Subscribe("latency.control.schedule", agent.handleScheduleCommand)
	if err != nil {
		log.Fatalf("subscribe schedule: %v", err)
	}
	defer scheduleSub.Unsubscribe()

	// Subscribe to targeted probe commands
	targetSub, err := nc.Subscribe(fmt.Sprintf("latency.control.probe.%s", cfg.Region), agent.handleTargetedProbe)
	if err != nil {
		log.Fatalf("subscribe targeted: %v", err)
	}
	defer targetSub.Unsubscribe()

	// HTTP ping endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", agent.handlePing)
	mux.HandleFunc("/health", agent.handleHealth)

	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	// Start HTTP server
	go func() {
		log.Printf("HTTP listening on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	// Publish initial health
	agent.publishHealth()

	// Health ticker
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				agent.publishHealth()
			case <-agent.done:
				return
			}
		}
	}()

	// Wait for shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Printf("shutting down")
	close(agent.done)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	nc.Drain()
}

// handleProbeRequest responds to latency probe requests immediately.
func (a *Agent) handleProbeRequest(msg *nats.Msg) {
	resp := map[string]interface{}{
		"region": a.cfg.Region,
		"ts":     time.Now().UnixNano(),
	}
	data, _ := json.Marshal(resp)
	msg.Respond(data)
}

// handleScheduleCommand triggers a full sweep when the hub says so.
func (a *Agent) handleScheduleCommand(msg *nats.Msg) {
	var cmd struct {
		Regions []string `json:"regions"`
	}
	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		log.Printf("bad schedule command: %v", err)
		return
	}

	// Stagger start based on region name hash to avoid burst
	hash := 0
	for _, c := range a.cfg.Region {
		hash = hash*31 + int(c)
	}
	if hash < 0 {
		hash = -hash
	}
	stagger := time.Duration(hash%30) * time.Second
	log.Printf("scheduled sweep in %v for %d targets", stagger, len(cmd.Regions))

	go func() {
		time.Sleep(stagger)
		a.runSweep(cmd.Regions)
	}()
}

// handleTargetedProbe measures latency to a specific target on demand.
func (a *Agent) handleTargetedProbe(msg *nats.Msg) {
	var cmd struct {
		Target string `json:"target"`
	}
	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		return
	}
	go func() {
		result := a.measureOne(cmd.Target)
		if result != nil {
			a.publishResult(result)
		}
	}()
}

// runSweep measures latency to all target regions sequentially.
func (a *Agent) runSweep(targets []string) {
	log.Printf("starting sweep of %d targets", len(targets))
	start := time.Now()
	measured := 0

	for _, target := range targets {
		if target == a.cfg.Region {
			continue
		}
		result := a.measureOne(target)
		if result != nil {
			a.publishResult(result)
			measured++
		}
	}

	log.Printf("sweep complete: %d/%d measured in %v", measured, len(targets)-1, time.Since(start))
}

type probeResult struct {
	Source    string    `json:"source"`
	Target    string    `json:"target"`
	RTTMs     float64   `json:"rtt_ms"`
	MinMs     float64   `json:"min_ms"`
	MaxMs     float64   `json:"max_ms"`
	P50Ms     float64   `json:"p50_ms"`
	Samples   int       `json:"samples"`
	Timestamp time.Time `json:"timestamp"`
}

// measureOne sends multiple NATS request/reply probes to a target and returns stats.
func (a *Agent) measureOne(target string) *probeResult {
	subject := fmt.Sprintf("latency.probe.request.%s", target)
	const numSamples = 5
	const timeout = 3 * time.Second

	var rtts []float64
	for i := 0; i < numSamples; i++ {
		start := time.Now()
		_, err := a.nc.Request(subject, nil, timeout)
		elapsed := time.Since(start)
		if err != nil {
			continue
		}
		rtts = append(rtts, float64(elapsed.Microseconds())/1000.0)
	}

	if len(rtts) == 0 {
		return nil
	}

	sort.Float64s(rtts)
	minVal := rtts[0]
	maxVal := rtts[len(rtts)-1]
	p50 := rtts[len(rtts)/2]

	sum := 0.0
	for _, v := range rtts {
		sum += v
	}
	avg := sum / float64(len(rtts))

	return &probeResult{
		Source:    a.cfg.Region,
		Target:    target,
		RTTMs:     math.Round(avg*100) / 100,
		MinMs:     math.Round(minVal*100) / 100,
		MaxMs:     math.Round(maxVal*100) / 100,
		P50Ms:     math.Round(p50*100) / 100,
		Samples:   len(rtts),
		Timestamp: time.Now(),
	}
}

func (a *Agent) publishResult(r *probeResult) {
	subject := fmt.Sprintf("latency.results.%s.%s", r.Source, r.Target)
	data, _ := json.Marshal(r)
	if err := a.nc.Publish(subject, data); err != nil {
		log.Printf("publish result: %v", err)
	}
}

func (a *Agent) publishHealth() {
	health := map[string]interface{}{
		"region": a.cfg.Region,
		"status": "ok",
		"ts":     time.Now().Unix(),
	}
	data, _ := json.Marshal(health)
	a.nc.Publish(fmt.Sprintf("latency.health.%s", a.cfg.Region), data)
}

// HTTP handlers

func (a *Agent) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"region": a.cfg.Region,
		"ts":     time.Now().UnixMilli(),
	})
}

func (a *Agent) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	connected := a.nc.IsConnected()
	status := "ok"
	if !connected {
		status = "degraded"
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"region":         a.cfg.Region,
		"status":         status,
		"nats_connected": connected,
		"ts":             time.Now().Unix(),
	})
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
