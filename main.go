package main

import (
	"encoding/json"
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

func validate_chirp(w http.ResponseWriter, r *http.Request) {
    type errors struct {
        Error string `json:"error"`
    }

    type parameters struct {
        Body string `json:"body"`
    }

    decoder := json.NewDecoder(r.Body)
    params := parameters{}
    err := decoder.Decode(&params)
    if err != nil {
        fmt.Printf("Error decoding parameters: %s", err)
        w.WriteHeader(500)
        respError := errors{
            Error: "Something went wrong",
        }
        encodedError, err := json.Marshal(respError)
        if err != nil {
            fmt.Printf("Error encoding Error JSON: %s", err)
            return
        }
        w.Write(encodedError)
        return
    }

    if len(params.Body) > 140 {
        w.WriteHeader(400)
        respError := errors{
            Error: "Chirp is too long",
        }
        encodedError, err := json.Marshal(respError)
        if err != nil {
            fmt.Printf("Error encoding Error JSON: %s", err)
            return
        }
        w.Write(encodedError)
        return

    }

    type returnVals struct {
        Valid bool `json:"valid"`
    }

    respBody := returnVals{
        Valid: true,
    }

    dat, err := json.Marshal(respBody)
	if err != nil {
			fmt.Printf("Error marshalling JSON: %s", err)
			w.WriteHeader(500)
			return
	}

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(200)
    w.Write(dat)
}

// view count handler
func (cfg *apiConfig) views(w http.ResponseWriter, r *http.Request) {
    r.Header.Set("Content-Type", "text/html")
    content := fmt.Sprintf(`
    <html>
    <body>
        <h1>Welcome, Chirpy Admin</h1>
        <p>Chirpy has been visited %d times!</p>
    </body>
    </html>
    `, cfg.fileserverHits.Load())

    w.Write([]byte(content))
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
    mux.HandleFunc("GET /admin/metrics", apiCfg.views)
    mux.HandleFunc("POST /admin/reset", apiCfg.reset)

    mux.Handle("/app/", apiCfg.middlewareMetricsInc(handler))
    mux.Handle("/assets", http.FileServer(http.Dir("./assets/logo.png")))

    // validate chirp
    mux.HandleFunc("POST /api/validate_chirp", validate_chirp)

    if err := server.ListenAndServe(); err != nil {
        fmt.Printf("error: %v", err)
    }
}
