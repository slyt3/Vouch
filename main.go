package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/slyt3/Vouch/internal/api"
	"github.com/slyt3/Vouch/internal/assert"
	"github.com/slyt3/Vouch/internal/core"
	"github.com/slyt3/Vouch/internal/interceptor"
	"github.com/slyt3/Vouch/internal/ledger"
	"github.com/slyt3/Vouch/internal/ledger/store"
	"github.com/slyt3/Vouch/internal/observer"
)

const (
	adminAddr       = ":9998"
	shutdownTimeout = 10 * time.Second
)

func main() {
	configPath := flag.String("config", "vouch-policy.yaml", "path to policy configuration")
	target := flag.String("target", "http://localhost:8080", "target tool server URL")
	listenPort := flag.Int("port", 9999, "port to listen on")
	flag.Parse()

	if err := assert.Check(*target != "", "target must not be empty"); err != nil {
		log.Fatalf("Invalid target: %v", err)
	}
	if err := assert.Check(*listenPort > 0, "listen port must be positive"); err != nil {
		log.Fatalf("Invalid listen port: %v", err)
	}

	// 1. Load Observer Rules
	obsEngine, err := observer.NewObserverEngine(*configPath)
	if err != nil {
		log.Fatalf("Failed to load observer rules: %v", err)
	}
	obsEngine.Watch()

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

	wrappedProxy := buildProxyHandler(interceptorSvc, reverseProxy)
	adminServer := newAdminServer(apiHandlers)
	proxyServer := newProxyServer(*listenPort, wrappedProxy)

	log.Printf("Admin API: %s", adminAddr)
	startHTTPServer(adminServer, "Admin API")
	log.Printf("Proxy Server: :%d -> %s", *listenPort, *target)
	startHTTPServer(proxyServer, "Proxy Server")

	shutdownSignal := waitForShutdownSignal(syscall.SIGINT, syscall.SIGTERM)
	log.Printf("Shutdown signal received: %v", shutdownSignal)
	gracefulShutdown(obsEngine, worker, adminServer, proxyServer, shutdownTimeout)
}

func buildProxyHandler(interceptorSvc *interceptor.Interceptor, reverseProxy *httputil.ReverseProxy) http.Handler {
	if err := assert.NotNil(interceptorSvc, "interceptor"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	}
	if err := assert.NotNil(reverseProxy, "reverse proxy"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		interceptorSvc.InterceptRequest(r)
		reverseProxy.ServeHTTP(w, r)
	})
}

func newAdminServer(apiHandlers *api.Handlers) *http.Server {
	if err := assert.NotNil(apiHandlers, "api handlers"); err != nil {
		return &http.Server{}
	}
	if err := assert.Check(adminAddr != "", "admin addr must not be empty"); err != nil {
		return &http.Server{}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/rekey", apiHandlers.HandleRekey)
	mux.HandleFunc("/api/metrics", apiHandlers.HandleStats)
	mux.HandleFunc("/metrics", apiHandlers.HandlePrometheus)
	mux.HandleFunc("/healthz", apiHandlers.HandleHealth)
	mux.HandleFunc("/readyz", apiHandlers.HandleReady)

	return &http.Server{Addr: adminAddr, Handler: mux}
}

func newProxyServer(port int, handler http.Handler) *http.Server {
	if err := assert.Check(port > 0, "port must be positive"); err != nil {
		return &http.Server{}
	}
	if err := assert.NotNil(handler, "handler"); err != nil {
		return &http.Server{}
	}

	addr := fmt.Sprintf(":%d", port)
	return &http.Server{Addr: addr, Handler: handler}
}

func startHTTPServer(server *http.Server, label string) {
	if err := assert.NotNil(server, "server"); err != nil {
		return
	}
	if err := assert.Check(label != "", "label must not be empty"); err != nil {
		return
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("%s error: %v", label, err)
		}
	}()
}

func shutdownHTTPServer(server *http.Server, timeout time.Duration, label string) {
	if err := assert.NotNil(server, "server"); err != nil {
		return
	}
	if err := assert.Check(timeout > 0, "timeout must be positive"); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("[WARN] %s shutdown failed: %v", label, err)
	}
}

func waitForShutdownSignal(signals ...os.Signal) os.Signal {
	if err := assert.Check(len(signals) > 0, "signals must not be empty"); err != nil {
		return nil
	}
	sigCh := make(chan os.Signal, 1)
	if err := assert.NotNil(sigCh, "signal channel"); err != nil {
		return nil
	}

	signal.Notify(sigCh, signals...)
	return <-sigCh
}

func gracefulShutdown(obsEngine *observer.ObserverEngine, worker *ledger.Worker, adminServer *http.Server, proxyServer *http.Server, timeout time.Duration) {
	if err := assert.NotNil(obsEngine, "observer engine"); err != nil {
		return
	}
	if err := assert.NotNil(worker, "worker"); err != nil {
		return
	}
	if err := assert.NotNil(adminServer, "admin server"); err != nil {
		return
	}
	if err := assert.NotNil(proxyServer, "proxy server"); err != nil {
		return
	}
	if err := assert.Check(timeout > 0, "timeout must be positive"); err != nil {
		return
	}

	shutdownHTTPServer(proxyServer, timeout, "Proxy Server")
	shutdownHTTPServer(adminServer, timeout, "Admin API")

	if err := obsEngine.Stop(); err != nil {
		log.Printf("[WARN] observer stop failed: %v", err)
	}
	if err := worker.Shutdown(timeout); err != nil {
		log.Printf("[WARN] worker shutdown failed: %v", err)
	}
}
