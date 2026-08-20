// Package cache implements per-tenant TTL caches with hard size bounds.
// Each tenant gets an isolated namespace; keys from one tenant are invisible
// to another, preventing cross-tenant data leakage even in-process.
package cache

import (
	"sync"
	"time"
)

const (
	defaultMaxEntries = 2000
	defaultTTL        = 5 * time.Minute
)

type entry struct {
	value     any
	expiresAt time.Time
}

// TenantCache is a per-tenant TTL cache with bounded capacity.
type TenantCache struct {
	mu      sync.RWMutex
	entries map[string]entry
	maxSize int
}

func newTenantCache(maxSize int) *TenantCache {
	return &TenantCache{
		entries: make(map[string]entry, maxSize/4),
		maxSize: maxSize,
	}
}

func (c *TenantCache) get(key string) (any, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.value, true
}

func (c *TenantCache) set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxSize {
		// Evict one expired entry (or any entry if none are expired)
		for k, e := range c.entries {
			if time.Now().After(e.expiresAt) {
				delete(c.entries, k)
				break
			}
		}
		if len(c.entries) >= c.maxSize {
			for k := range c.entries {
				delete(c.entries, k)
				break
			}
		}
	}
	c.entries[key] = entry{value: value, expiresAt: time.Now().Add(ttl)}
}

func (c *TenantCache) evictAll() {
	c.mu.Lock()
	c.entries = make(map[string]entry)
	c.mu.Unlock()
}

// Manager manages per-tenant cache instances.
// Each tenant gets its own isolated TenantCache.
type Manager struct {
	mu       sync.RWMutex
	caches   map[string]*TenantCache
	maxPerTenant int
}

// NewManager creates a cache Manager. maxPerTenant is the max entries per tenant cache.
func NewManager(maxPerTenant int) *Manager {
	if maxPerTenant <= 0 {
		maxPerTenant = defaultMaxEntries
	}
	return &Manager{
		caches:       make(map[string]*TenantCache),
		maxPerTenant: maxPerTenant,
	}
}

func (m *Manager) cache(tenantID string) *TenantCache {
	m.mu.RLock()
	c, ok := m.caches[tenantID]
	m.mu.RUnlock()
	if ok {
		return c
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok = m.caches[tenantID]; ok {
		return c
	}
	c = newTenantCache(m.maxPerTenant)
	m.caches[tenantID] = c
	return c
}

// Get retrieves a value from the tenant's cache.
// The caller receives only their own tenant's data — cross-tenant access is impossible
// because tenantID is always the partition key.
func (m *Manager) Get(tenantID, key string) (any, bool) {
	return m.cache(tenantID).get(key)
}

// Set stores a value in the tenant's cache with the given TTL.
func (m *Manager) Set(tenantID, key string, value any, ttl time.Duration) {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	m.cache(tenantID).set(key, value, ttl)
}

// Evict drops the entire cache for a tenant (called on suspension or disable).
func (m *Manager) Evict(tenantID string) {
	m.mu.RLock()
	c, ok := m.caches[tenantID]
	m.mu.RUnlock()
	if ok {
		c.evictAll()
	}
}

// Stats returns the number of non-expired entries in the tenant's cache.
func (m *Manager) Stats(tenantID string) int {
	m.mu.RLock()
	c, ok := m.caches[tenantID]
	m.mu.RUnlock()
	if !ok {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	live := 0
	now := time.Now()
	for _, e := range c.entries {
		if !now.After(e.expiresAt) {
			live++
		}
	}
	return live
}
