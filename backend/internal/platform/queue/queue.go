// Package queue implements per-tenant bounded ingestion queues with a shared
// worker pool. Noisy tenants fill only their own queue and are subject to
// their own capacity; they cannot starve other tenants' processing.
package queue

import (
	"sync"
	"sync/atomic"
	"time"
)

// Job is a unit of work enqueued by an ingestion handler.
type Job struct {
	TenantID  string
	Payload   []byte
	Meta      map[string]string
	EnqueuedAt time.Time
}

// Processor is the function called by a worker for each dequeued job.
type Processor func(job Job)

const (
	defaultCapacity = 500  // items per tenant queue
	defaultWorkers  = 8    // shared worker goroutines
)

type tenantQueue struct {
	ch      chan Job
	dropped atomic.Int64
}

// Manager holds per-tenant queues and a shared worker pool.
type Manager struct {
	mu        sync.RWMutex
	queues    map[string]*tenantQueue
	capacity  int
	processor Processor
	work      chan Job // fan-in channel fed by all tenant queues
	once      sync.Once
}

// New creates a Manager. Call Start() to launch the worker pool.
func New(capacity, workers int, processor Processor) *Manager {
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	if workers <= 0 {
		workers = defaultWorkers
	}
	m := &Manager{
		queues:    make(map[string]*tenantQueue),
		capacity:  capacity,
		processor: processor,
		work:      make(chan Job, capacity*4),
	}
	m.once.Do(func() {
		for i := 0; i < workers; i++ {
			go m.runWorker()
		}
	})
	return m
}

// Enqueue adds a job to the tenant's queue.
// Returns false and increments the dropped counter when the queue is full.
func (m *Manager) Enqueue(job Job) bool {
	q := m.getOrCreate(job.TenantID)
	job.EnqueuedAt = time.Now()
	select {
	case q.ch <- job:
		// Forward to shared fan-in immediately
		select {
		case m.work <- job:
		default:
		}
		return true
	default:
		q.dropped.Add(1)
		return false
	}
}

// DroppedCount returns the number of jobs dropped for a tenant since startup.
func (m *Manager) DroppedCount(tenantID string) int64 {
	m.mu.RLock()
	q, ok := m.queues[tenantID]
	m.mu.RUnlock()
	if !ok {
		return 0
	}
	return q.dropped.Load()
}

// Depth returns the current queue depth for a tenant.
func (m *Manager) Depth(tenantID string) int {
	m.mu.RLock()
	q, ok := m.queues[tenantID]
	m.mu.RUnlock()
	if !ok {
		return 0
	}
	return len(q.ch)
}

// Stats returns queue depth and dropped count for all active tenants.
func (m *Manager) Stats() map[string]map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]map[string]int64, len(m.queues))
	for id, q := range m.queues {
		out[id] = map[string]int64{
			"depth":   int64(len(q.ch)),
			"dropped": q.dropped.Load(),
		}
	}
	return out
}

// Purge drains and removes the queue for a tenant (on suspension/disable).
func (m *Manager) Purge(tenantID string) {
	m.mu.Lock()
	q, ok := m.queues[tenantID]
	if ok {
		delete(m.queues, tenantID)
	}
	m.mu.Unlock()
	if ok {
		for len(q.ch) > 0 {
			<-q.ch
		}
	}
}

func (m *Manager) runWorker() {
	for job := range m.work {
		m.processor(job)
	}
}

// SetProcessor replaces the active job processor.
// Safe to call after New() — useful when the processor depends on services
// that are initialised after the queue (e.g. correlation service).
// The swap is protected by the same mutex used for tenant-queue creation
// so in-flight jobs complete with the old processor.
func (m *Manager) SetProcessor(p Processor) {
	m.mu.Lock()
	m.processor = p
	m.mu.Unlock()
}

func (m *Manager) getOrCreate(tenantID string) *tenantQueue {
	m.mu.RLock()
	q, ok := m.queues[tenantID]
	m.mu.RUnlock()
	if ok {
		return q
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if q, ok = m.queues[tenantID]; ok {
		return q
	}
	q = &tenantQueue{ch: make(chan Job, m.capacity)}
	m.queues[tenantID] = q
	return q
}
