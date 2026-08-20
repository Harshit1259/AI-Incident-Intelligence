package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// WebhookRateLimiter enforces per-token request rate limits on ingest endpoints.
// Uses a token-bucket algorithm. Safe for concurrent use. No external dependencies.
type WebhookRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	limit   int
	window  time.Duration
}

type tokenBucket struct {
	tokens     int
	lastSeen   time.Time
	nextRefill time.Time
}

// NewWebhookRateLimiter creates a limiter: limit requests per window per key.
// Typical call: NewWebhookRateLimiter(100, time.Minute).
func NewWebhookRateLimiter(limit int, window time.Duration) *WebhookRateLimiter {
	rl := &WebhookRateLimiter{
		buckets: make(map[string]*tokenBucket),
		limit:   limit,
		window:  window,
	}
	go rl.cleanup()
	return rl
}

// Allow returns true if the key has remaining quota in the current window.
func (rl *WebhookRateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.buckets[key]
	if !exists {
		rl.buckets[key] = &tokenBucket{
			tokens:     rl.limit - 1,
			lastSeen:   now,
			nextRefill: now.Add(rl.window),
		}
		return true
	}

	b.lastSeen = now
	if now.After(b.nextRefill) {
		b.tokens = rl.limit
		b.nextRefill = now.Add(rl.window)
	}

	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}

// Middleware wraps a HandlerFunc with rate limiting.
// Key priority: X-Source-Token → X-Forwarded-For → RemoteAddr.
func (rl *WebhookRateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := rateLimitKey(r)
		if !rl.Allow(key) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded — try again later"}`)) //nolint:errcheck
			return
		}
		next(w, r)
	}
}

// rateLimitKey returns the most specific stable identifier for the caller.
func rateLimitKey(r *http.Request) string {
	if tok := r.Header.Get("X-Source-Token"); tok != "" {
		return "tok:" + tok
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// Use only the first IP — the originating client.
		return "ip:" + strings.SplitN(fwd, ",", 2)[0]
	}
	// RemoteAddr includes port; strip it for stability behind NAT.
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		addr = addr[:idx]
	}
	return "ip:" + addr
}

// cleanup removes buckets that have been idle for 10 minutes.
func (rl *WebhookRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-10 * time.Minute)
		rl.mu.Lock()
		for key, b := range rl.buckets {
			if b.lastSeen.Before(cutoff) {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}
