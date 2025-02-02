package main

import (
	"fmt"
	"net/http"
)

func main() {
    mux := http.NewServeMux()
    server := &http.Server{
        Addr:       ":8080",
        Handler:    mux,
    }

    mux.Handle("/", http.FileServer(http.Dir(".")))
    mux.Handle("/assets", http.FileServer(http.Dir("./assets/logo.png")))

    if err := server.ListenAndServe(); err != nil {
        fmt.Printf("error: %v", err)
    }
}
