package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/slyt3/Vouch/internal/api"
	"github.com/slyt3/Vouch/internal/core"
	"github.com/slyt3/Vouch/internal/interceptor"
	"github.com/slyt3/Vouch/internal/ledger"
	"github.com/slyt3/Vouch/internal/ledger/store"
	"github.com/slyt3/Vouch/internal/observer"
)

func main() {
	configPath := flag.String("config", "vouch-policy.yaml", "path to policy configuration")
	target := flag.String("target", "http://localhost:8080", "target tool server URL")
	listenPort := flag.Int("port", 9999, "port to listen on")
	flag.Parse()

	// 1. Load Observer Rules
	obsEngine, err := observer.NewObserverEngine(*configPath)
	if err != nil {
		log.Fatalf("Failed to load observer rules: %v", err)
	}

	// 2. Initialize Ledger Store & Worker
	db, err := store.NewDB("vouch.db")
	if err != nil {
		log.Fatalf("Database init failed: %v", err)
	}
	worker, err := ledger.NewWorker(1000, db, ".vouch_key")
	if err != nil {
		log.Fatalf("Worker init failed: %v", err)
	}
	if err := worker.Start(); err != nil {
		log.Fatalf("Worker start failed: %v", err)
	}

	// 3. Initialize Core Engine
	engine := core.NewEngine(worker, obsEngine)

	// 4. Initialize Interceptor
	interceptorSvc := interceptor.NewInterceptor(engine)

	// 5. Initialize API Handlers
	apiHandlers := api.NewHandlers(engine)

	// 6. Setup Proxy
	targetURL, err := url.Parse(*target)
	if err != nil {
		log.Fatalf("Invalid target URL: %v", err)
	}
	reverseProxy := httputil.NewSingleHostReverseProxy(targetURL)
	reverseProxy.ModifyResponse = interceptorSvc.InterceptResponse

	// Wrap proxy with interceptor
	wrappedProxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		interceptorSvc.InterceptRequest(r)
		reverseProxy.ServeHTTP(w, r)
	})

	// 7. Start API Server
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/rekey", apiHandlers.HandleRekey)
		mux.HandleFunc("/api/metrics", apiHandlers.HandleStats)
		log.Print("Admin API: :9998")
		if err := http.ListenAndServe(":9998", mux); err != nil {
			log.Fatalf("API server error: %v", err)
		}
	}()

	// 8. Start Proxy Server
	log.Printf("Proxy Server: :%d -> %s", *listenPort, *target)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *listenPort), wrappedProxy); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
