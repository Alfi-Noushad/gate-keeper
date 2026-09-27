package main

import (
    "fmt"
    "log"
    "net/http"
    "net/http/httputil"
    "net/url"
    "strconv"
    "time"
    "context"
    _ "embed"

    "github.com/redis/go-redis/v9"
)

// Embed token_bucket.lua directly into the go binary
//go:embed token_bucket.lua
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
        Addr: "localhost:6379",
    })

    if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	fmt.Println("[GateKeeper] Connected to Redis successfully.")

	//luaScript := redis.NewScript(tokenBucketLuaScript)

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

    // Rate Limiter Check
    apiKey := r.Header.Get("X-API-Key")
    if apiKey == "" {
    apiKey = "anonymous_client"
    }

    redisKey := fmt.Sprintf("rate_limit:%s", apiKey)

    now := time.Now().Unix()

    luaScript := redis.NewScript(tokenBucketLuaScript)
    //fmt.Println("Lua script loaded, length:", len(tokenBucketLuaScript))

    res, err := luaScript.Run(
        r.Context(),
        rdb,
        []string{redisKey},
        5, // capacity
        1, // refill rate
        now,
        1, // tokens requested
    ).Result()

    if err != nil {
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        log.Printf("Redis execution error: %v", err)
        return
    }

    results := res.([]interface{})
    allowed := results[0].(int64)
    remainingTokens := results[1].(int64)

    w.Header().Set("X-RateLimit-Limit", strconv.Itoa(5))
    w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remainingTokens, 10))

    if allowed == 0 {
        w.Header().Set("Retry-After", "1")
        w.WriteHeader(http.StatusTooManyRequests)
        w.Write([]byte("429 Too Many Requests: Rate limit exceeded.\n"))
        
        fmt.Printf("[429 REJECTED] Client: %s | Remaining: %d\n", apiKey, remainingTokens)

        return
    }


        fmt.Printf("[200 ALLOWED]  Client: %s | Remaining: %d\n", apiKey, remainingTokens)
		// Forward the request to the downstream target
		proxy.ServeHTTP(w, r)
	})

    port := ":8080"
	fmt.Printf("GateKeeper running on http://localhost%s forwarding to %s\n", port, targetURL)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}