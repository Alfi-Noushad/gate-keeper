package main

import (
    "fmt"
    "log"
    "net/http"
    "net/http/httputil"
    "net/url"
)

// NewProxy creates an HTTP handler that forwards incoming traffic to a target URL.
func NewProxy(target string) (*httputil.ReverseProxy, error) {
    parsedURL, err := url.Parse(target)
    if err != nil {
        return nil, err
    }
    // go standard library for reverse proxy
    return httputil.NewSingleHostReverseProxy(parsedURL), nil
}

func main() {
    //fmt.Println("Hello GateKeeper")
    // downstram target
    targetURL := "https://httpbin.org"

    proxy, err := NewProxy(targetURL)
	if err != nil {
		log.Fatalf("Failed to initialize reverse proxy: %v", err)
	}
    // Define the http req handler
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("[GateKeeper] Intercepted %s request for: %s\n", r.Method, r.URL.Path)

		// todo: Insert Rate Limiter Check

		// Forward the request to the downstream target
		proxy.ServeHTTP(w, r)
	})

    port := ":8080"
	fmt.Printf("GateKeeper running on http://localhost%s forwarding to %s\n", port, targetURL)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}