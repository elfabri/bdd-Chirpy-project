package main

import (
	"fmt"
	"net/http"
)

// readiness handler
func readiness(resW http.ResponseWriter, req *http.Request) {
    req.Header.Set("Content-Type", "application/json")
    resW.WriteHeader(200)
    resW.Write([]byte("OK"))
}

func main() {
    mux := http.NewServeMux()
    server := &http.Server{
        Addr:       ":8080",
        Handler:    mux,
    }

    // readiness endpoint
    mux.HandleFunc("/healthz", readiness)

    mux.Handle("/app/", http.StripPrefix("/app/", http.FileServer(http.Dir("."))))
    mux.Handle("/assets", http.FileServer(http.Dir("./assets/logo.png")))

    if err := server.ListenAndServe(); err != nil {
        fmt.Printf("error: %v", err)
    }
}
