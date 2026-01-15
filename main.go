package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/slyt3/Vouch/internal/api"
	"github.com/slyt3/Vouch/internal/core"
	"github.com/slyt3/Vouch/internal/interceptor"
	"github.com/slyt3/Vouch/internal/ledger"
	"github.com/slyt3/Vouch/internal/proxy"
)

func main() {
	log.Println("Vouch (Agent Analytics & Safety) - Starting Monolithic -> Modular transition")

	// 1. Load Policy
	policy, err := proxy.LoadPolicy("vouch-policy.yaml")
	if err != nil {
		log.Fatalf("Policy load failed: %v", err)
	}

	// 2. Initialize Ledger Worker
	worker, err := ledger.NewWorker(1000, "vouch.db", ".vouch_key")
	if err != nil {
		log.Fatalf("Worker init failed: %v", err)
	}
	if err := worker.Start(); err != nil {
		log.Fatalf("Worker start failed: %v", err)
	}

	// 3. Initialize Core Engine
	engine := core.NewEngine(worker, policy)

	// 4. Initialize Interceptor
	interceptorSvc := interceptor.NewInterceptor(engine)

	// 5. Initialize API Handlers
	apiHandlers := api.NewHandlers(engine)

	// 6. Setup Proxy
	targetURL, _ := url.Parse("http://localhost:8080")
	reverseProxy := httputil.NewSingleHostReverseProxy(targetURL)

	originalDirector := reverseProxy.Director
	reverseProxy.Director = func(req *http.Request) {
		originalDirector(req)
		interceptorSvc.InterceptRequest(req)
	}
	reverseProxy.ModifyResponse = interceptorSvc.InterceptResponse

	// 7. Start API Server
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/approve/", apiHandlers.HandleApprove)
		mux.HandleFunc("/api/reject/", apiHandlers.HandleReject)
		mux.HandleFunc("/api/rekey", apiHandlers.HandleRekey)
		mux.HandleFunc("/api/metrics", apiHandlers.HandleStats)
		log.Print("Admin API: :9998")
		if err := http.ListenAndServe(":9998", mux); err != nil {
			log.Fatalf("API server error: %v", err)
		}
	}()

	// 8. Start Proxy Server
	log.Print("Proxy Server: :9999 -> :8080")
	if err := http.ListenAndServe(":9999", reverseProxy); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
