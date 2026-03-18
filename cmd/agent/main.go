package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

func main() {
	region := os.Getenv("REGION")
	if region == "" {
		log.Fatal("REGION required")
	}
	seedRoutes := os.Getenv("NATS_SEED_ROUTES") // nats-route://hub:6222
	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":443"
	}
	tlsCert := os.Getenv("TLS_CERT") // /etc/latency/fullchain.pem
	tlsKey := os.Getenv("TLS_KEY")   // /etc/latency/privkey.pem
	clusterPort := 6222
	monitorPort := 8222
	clientPort := 4222

	log.Printf("agent starting: region=%s seed=%s", region, seedRoutes)

	// Parse seed route URLs
	var routes []*url.URL
	if seedRoutes != "" {
		routes = server.RoutesFromStr(seedRoutes)
	}

	// Start embedded NATS server as cluster member
	opts := &server.Options{
		ServerName: region,
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

	// HTTP server for /ping (client latency measurement) and /health
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"region": region,
			"ts":     time.Now().UnixMilli(),
		})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		numRoutes := ns.NumRoutes()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"region":     region,
			"status":     "ok",
			"num_routes": numRoutes,
			"ts":         time.Now().Unix(),
		})
	})

	httpServer := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
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

	// Log route count periodically
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			log.Printf("cluster routes: %d", ns.NumRoutes())
		}
	}()

	// Wait for shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Printf("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
	ns.Shutdown()
}
