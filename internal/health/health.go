// Package health probes internal targets for reachability and caches the result.
//
// The query path must never block on a probe: off-VPN a connect to an RFC1918
// address stalls until timeout, which would hang DNS resolution. Callers read
// the cached state only; probing happens in a background loop.
package health

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// Target identifies a distinct host:port that gets health-checked.
type Target struct {
	IP   string
	Port int
}

func (t Target) addr() string { return net.JoinHostPort(t.IP, fmt.Sprint(t.Port)) }

// Checker tracks reachability of a set of targets.
type Checker struct {
	timeout time.Duration

	mu      sync.RWMutex
	targets map[Target]struct{}
	up      map[Target]bool
}

// New returns a Checker probing with the given dial timeout.
func New(timeout time.Duration) *Checker {
	return &Checker{
		timeout: timeout,
		targets: make(map[Target]struct{}),
		up:      make(map[Target]bool),
	}
}

// SetTargets replaces the set of probed targets. Unknown targets start as down
// until the next probe round confirms them.
func (c *Checker) SetTargets(ts []Target) {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := make(map[Target]struct{}, len(ts))
	for _, t := range ts {
		next[t] = struct{}{}
	}
	c.targets = next
	// Drop stale state for targets no longer configured.
	for t := range c.up {
		if _, ok := next[t]; !ok {
			delete(c.up, t)
		}
	}
}

// Up reports the cached reachability of a target. Unknown targets read as down.
func (c *Checker) Up(t Target) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.up[t]
}

// Run probes all targets every interval until stop is closed. It performs one
// round immediately so state is fresh shortly after startup.
func (c *Checker) Run(interval time.Duration, stop <-chan struct{}) {
	c.probeAll()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			c.probeAll()
		}
	}
}

func (c *Checker) probeAll() {
	c.mu.RLock()
	targets := make([]Target, 0, len(c.targets))
	for t := range c.targets {
		targets = append(targets, t)
	}
	timeout := c.timeout
	c.mu.RUnlock()

	results := make(map[Target]bool, len(targets))
	var wg sync.WaitGroup
	var rmu sync.Mutex
	for _, t := range targets {
		wg.Add(1)
		go func(t Target) {
			defer wg.Done()
			ok := probe(t.addr(), timeout)
			rmu.Lock()
			results[t] = ok
			rmu.Unlock()
		}(t)
	}
	wg.Wait()

	c.mu.Lock()
	for t, ok := range results {
		if _, still := c.targets[t]; still {
			c.up[t] = ok
		}
	}
	c.mu.Unlock()
}

// probe returns true if a TCP connection to addr succeeds within timeout.
func probe(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
