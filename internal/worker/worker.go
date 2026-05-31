package worker

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Config holds the worker pool configuration.
type Config struct {
	WorkerCount int
	Timeout     time.Duration
	MaxRetries  int
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		WorkerCount: 4,
		Timeout:     10 * time.Second,
		MaxRetries:  2,
	}
}

// Conn represents a single connection to a JS worker process.
type Conn struct {
	ID       uint32
	Conn     net.Conn
	Busy     bool
	LastUsed time.Time
	Cmd      *process // nil when using mock connections
	pool     *Pool
}

// process abstracts the worker OS process for testability.
type process struct {
	cancel func()
}

// Pool manages a pool of worker connections with round-robin dispatch.
type Pool struct {
	config  Config
	workers []*Conn
	next    uint32
	mu      sync.Mutex
	stopped atomic.Bool
}

// NewPool creates a worker pool with the given config and connections.
// conns should be one per worker, already connected.
func NewPool(config Config) *Pool {
	return &Pool{
		config:  config,
		workers: make([]*Conn, 0),
	}
}

// AddConn adds an already-connected worker to the pool.
func (p *Pool) AddConn(id uint32, conn net.Conn) *Conn {
	w := &Conn{
		ID:       id,
		Conn:     conn,
		LastUsed: time.Now(),
		pool:     p,
	}
	p.mu.Lock()
	p.workers = append(p.workers, w)
	p.mu.Unlock()
	return w
}

// Acquire returns the next available (non-busy) worker using round-robin.
// Blocks until a worker becomes available or context is cancelled.
func (p *Pool) Acquire(ctx context.Context) (*Conn, error) {
	if p.stopped.Load() {
		return nil, fmt.Errorf("worker: pool is stopped")
	}

	for {
		p.mu.Lock()
		n := len(p.workers)
		if n == 0 {
			p.mu.Unlock()
			return nil, fmt.Errorf("worker: no workers in pool")
		}

		start := p.next
		for i := uint32(0); i < uint32(n); i++ {
			idx := (start + i) % uint32(n)
			w := p.workers[idx]
			if !w.Busy {
				w.Busy = true
				p.next = (idx + 1) % uint32(n)
				p.mu.Unlock()
				return w, nil
			}
		}
		p.mu.Unlock()

		// All workers busy — wait and retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// Release marks a worker as available for dispatch.
func (p *Pool) Release(w *Conn) {
	p.mu.Lock()
	w.Busy = false
	w.LastUsed = time.Now()
	p.mu.Unlock()
}

// MarkDead removes a dead worker and closes its connection.
// Returns true if the worker was found and removed.
func (p *Pool) MarkDead(w *Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, ww := range p.workers {
		if ww == w {
			p.workers = append(p.workers[:i], p.workers[i+1:]...)
			w.Conn.Close()
			return true
		}
	}
	return false
}

// Shutdown closes all worker connections.
func (p *Pool) Shutdown() error {
	p.stopped.Store(true)
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error
	for _, w := range p.workers {
		if w.Cmd != nil {
			w.Cmd.cancel()
		}
		if err := w.Conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	p.workers = nil

	if len(errs) > 0 {
		return fmt.Errorf("worker: shutdown errors: %v", errs)
	}
	return nil
}

// Len returns the current number of workers.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.workers)
}
