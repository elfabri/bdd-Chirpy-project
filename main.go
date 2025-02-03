package main

import (
	"fmt"
	"net/http"

	// "structs"
	"sync/atomic"
)

// stateful handlers
type apiConfig struct {
    // atomic for safely increment
    // and read an integer value
    // across multiple goroutines (HTTP requests)
	fileserverHits atomic.Int32
}

// middleware to count views
func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {

    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        cfg.fileserverHits.Add(1)
        next.ServeHTTP(w, r)
    })
}

// readiness handler
func readiness(resW http.ResponseWriter, req *http.Request) {
    req.Header.Set("Content-Type", "application/json")
    resW.WriteHeader(200)
    resW.Write([]byte("OK"))
}

// view count handler
func (cfg *apiConfig) views(resW http.ResponseWriter, _ *http.Request) {
    resW.Write([]byte(fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())))
}

// view count reset
func (cfg *apiConfig) reset(_ http.ResponseWriter, _ *http.Request) {
    cfg.fileserverHits.Store(0)
}

func main() {
    mux := http.NewServeMux()
    server := &http.Server{
        Addr:       ":8080",
        Handler:    mux,
    }
    apiCfg := apiConfig {fileserverHits: atomic.Int32{}}

    // handler main page
    handler := http.StripPrefix("/app/", http.FileServer(http.Dir(".")))

    // readiness endpoint
    mux.HandleFunc("GET /api/healthz", readiness)

    // view couter show & reset
    mux.HandleFunc("GET /api/metrics", apiCfg.views)
    mux.HandleFunc("POST /api/reset", apiCfg.reset)

    mux.Handle("/app/", apiCfg.middlewareMetricsInc(handler))
    mux.Handle("/assets", http.FileServer(http.Dir("./assets/logo.png")))

    if err := server.ListenAndServe(); err != nil {
        fmt.Printf("error: %v", err)
    }
}
