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
    "strings"

    "github.com/redis/go-redis/v9"
)

// Embed token_bucket.lua directly into the go binary
//go:embed token_bucket.lua
var tokenBucketLuaScript string

var ctx = context.Background()

//// Route represents a downstream target service mapped to a path prefix
type Route struct {
    Prefix      string
    TargetURL   *url.URL
    Proxy       *httputil.ReverseProxy
    StripPrefix bool
}

//collection fo routes
type Router struct {
    routes []Route
}

// Registering routes
func (r *Router) AddRoute(prefix string, target string, stripPrefix bool) error {
    //parse the target url
    parsedURL, err := url.Parse(target)
    if err != nil {
        return fmt.Errorf("invalid target URL '%s': %w", target, err)
    }

    proxy := httputil.NewSingleHostReverseProxy(parsedURL)

    //stripPrefix logic   GET /api/v1/users/123 ---> /123 is passed to backend
    if stripPrefix {
        originalDirector := proxy.Director

        proxy.Director = func(req *http.Request) {
            originalDirector(req)

            req.URL.Path = strings.TrimPrefix(
                req.URL.Path,
                prefix,
            )

            if !strings.HasPrefix(req.URL.Path, "/") {
                req.URL.Path = "/" + req.URL.Path
                }
        }    
    }

    r.routes = append(r.routes, Route{
    Prefix:      prefix,
    TargetURL:   parsedURL,
    Proxy:       proxy,
    StripPrefix: stripPrefix,
    })

   return nil
}

// finding the correct route
func (r *Router) Match(path string) *httputil.ReverseProxy {
    for _, route := range r.routes {
        if strings.HasPrefix(path, route.Prefix) {
            return route.Proxy
        }
    }

    return nil
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

	luaScript := redis.NewScript(tokenBucketLuaScript)

    // config dynamic routes
    router := &Router{}

    if err := router.AddRoute(
        "/api/v1/users",
        "https://httpbin.org/anything/users",
        true,
    ); err != nil {
        log.Fatalf("Failed to add users route: %v", err)
    }

    if err := router.AddRoute(
        "/api/v1/orders",
        "https://httpbin.org/anything/orders",
        true,
    ); err != nil {
        log.Fatalf("Failed to add orders route: %v", err)
    }

    if err := router.AddRoute(
        "/",
        "https://httpbin.org",
        false,
    ); err != nil {
        log.Fatalf("Failed to add default route: %v", err)
    }

    
    // Define the http req handler
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    fmt.Printf("[GateKeeper] Intercepted %s request for: %s\n", r.Method, r.URL.Path)
    
    proxy := router.Match(r.URL.Path)

    if proxy == nil {
        http.Error(w, "404 Route Not Found", http.StatusNotFound)
        return
    }

    // Rate Limiter Check
    apiKey := r.Header.Get("X-API-Key")
    if apiKey == "" {
    apiKey = "anonymous_client"
    }

    redisKey := fmt.Sprintf("rate_limit:%s", apiKey)

    now := time.Now().Unix()

    //luaScript := redis.NewScript(tokenBucketLuaScript)
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

	fmt.Printf(
    "GateKeeper running on http://localhost%s with dynamic routing\n",
    port,
    )
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}