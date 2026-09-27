package main

import (
    "fmt"
    "log"
    "net/http"
    "net/http/httputil"
    "net/url"
    "strconv"
    "time"

    "github.com/redis/go-redis/v9"
)

// Embed token_bucket.lua directly into the go binary
var tokenBucketLuaScript string

var ctx = context.Background()

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

    // connect to redis
    rdb := redis.NewClient(&redis.Options{
        Addr: "localhost:6379"
    })

    if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	fmt.Println("[GateKeeper] Connected to Redis successfully.")

	luaScript := redis.NewScript(tokenBucketLuaScript)

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