// Package ratelimit implements per-tenant token-bucket rate limiting.
// Each tenant gets independent buckets per category (ingest, query).
// Limits are loaded lazily from the LimitLoader function and refreshed every minute.
package ratelimit

import (
	"sync"
	"time"
)

// Category identifies which API surface is being rate-limited.
type Category string

const (
	CategoryIngest   Category = "ingest"
	CategoryQuery    Category = "query"
	CategoryAdmin    Category = "admin"
	CategoryMutation Category = "mutation"
)

// LimitLoader returns the per-minute limit for a tenant+category.
// Return 0 to indicate unlimited.
type LimitLoader func(tenantID string, cat Category) int

type bucket struct {
	mu       sync.Mutex
	tokens   float64
	lastFill time.Time
	rate     float64 // tokens per second
	limit    int     // tokens per minute (0 = unlimited)
	loadedAt time.Time
}

func (b *bucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limit == 0 {
		return true // unlimited
	}
	now := time.Now()
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens += elapsed * b.rate
	if b.tokens > float64(b.limit) {
		b.tokens = float64(b.limit)
	}
	b.lastFill = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// TenantRateLimiter holds per-tenant, per-category token buckets.
type TenantRateLimiter struct {
	loader  LimitLoader
	buckets sync.Map // key: tenantID+":"+category -> *bucket
}

// New creates a TenantRateLimiter backed by the given LimitLoader.
func New(loader LimitLoader) *TenantRateLimiter {
	return &TenantRateLimiter{loader: loader}
}

// Allow returns true if the request is within the tenant's rate limit.
// Returns true unconditionally when no limit is configured (unlimited plan).
func (l *TenantRateLimiter) Allow(tenantID string, cat Category) bool {
	key := tenantID + ":" + string(cat)

	raw, _ := l.buckets.LoadOrStore(key, &bucket{lastFill: time.Now()})
	b := raw.(*bucket)

	// Refresh limit every 60s to pick up plan changes.
	// On first use (loadedAt.IsZero), start the bucket full so the initial
	// burst of requests is not rejected immediately.
	b.mu.Lock()
	if time.Since(b.loadedAt) > 60*time.Second {
		isFirstLoad := b.loadedAt.IsZero()
		limit := l.loader(tenantID, cat)
		b.limit = limit
		b.rate = float64(limit) / 60.0
		if limit == 0 {
			b.rate = 0
		}
		b.loadedAt = time.Now()
		if isFirstLoad && limit > 0 {
			b.tokens = float64(limit) // start full on first use
		} else if b.tokens > float64(limit) && limit > 0 {
			b.tokens = float64(limit) // cap tokens when plan is downgraded
		}
	}
	b.mu.Unlock()

	return b.allow()
}

// Evict removes the rate-limit state for a tenant (called on suspension/disable).
func (l *TenantRateLimiter) Evict(tenantID string) {
	for _, cat := range []Category{CategoryIngest, CategoryQuery, CategoryAdmin, CategoryMutation} {
		l.buckets.Delete(tenantID + ":" + string(cat))
	}
}
