// Package resolver implements the split-horizon DNS handler: answer with the
// internal target when it is reachable, otherwise forward to an explicit
// upstream public resolver.
package resolver

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jarovkipt/lanfirst/internal/config"
	"github.com/jarovkipt/lanfirst/internal/health"
	"github.com/miekg/dns"
)

// Resolver answers DNS queries for the configured entries.
type Resolver struct {
	checker *health.Checker

	mu       sync.RWMutex
	entries  []config.Entry
	ttl      uint32
	upstream []string // explicit "ip:53" list; never points back at ourselves

	enabled atomic.Bool // when false, everything is forwarded upstream (LAN routing off)
}

// New builds a Resolver. upstream must be explicit servers, with the daemon's
// own listen address already excluded to avoid a resolution loop.
func New(c *config.Config, checker *health.Checker, upstream []string) *Resolver {
	r := &Resolver{checker: checker}
	r.enabled.Store(true)
	r.Update(c, upstream)
	return r
}

// Update swaps in new config (used on reload).
func (r *Resolver) Update(c *config.Config, upstream []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = c.Entries
	r.ttl = c.TTL
	r.upstream = upstream
}

// SetEnabled toggles LAN routing. When disabled, queries are forwarded upstream.
func (r *Resolver) SetEnabled(v bool) { r.enabled.Store(v) }

// Enabled reports whether LAN routing is active.
func (r *Resolver) Enabled() bool { return r.enabled.Load() }

// Mode describes the current routing decision for an entry, for the menu bar.
type Mode struct {
	Pattern string
	Target  string
	LAN     bool     // true = answering internal target, false = forwarding public
	Except  []string // hostnames/patterns under Pattern kept on public DNS
}

// Modes returns the current per-entry routing state.
func (r *Resolver) Modes() []Mode {
	r.mu.RLock()
	defer r.mu.RUnlock()
	enabled := r.enabled.Load()
	out := make([]Mode, 0, len(r.entries))
	for _, e := range r.entries {
		lan := enabled && r.checker.Up(health.Target{IP: e.Target, Port: e.Port})
		out = append(out, Mode{Pattern: e.Pattern, Target: e.Target, LAN: lan, Except: e.Except})
	}
	return out
}

// ServeDNS implements dns.Handler.
func (r *Resolver) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	if len(req.Question) == 0 {
		r.forward(w, req)
		return
	}
	q := req.Question[0]
	name := strings.TrimSuffix(strings.ToLower(q.Name), ".")

	r.mu.RLock()
	entry, ok := r.matchLocked(name)
	ttl := r.ttl
	enabled := r.enabled.Load()
	r.mu.RUnlock()

	// Not one of our domains, routing disabled, or target unreachable: forward.
	if !ok || !enabled || !r.checker.Up(health.Target{IP: entry.Target, Port: entry.Port}) {
		r.forward(w, req)
		return
	}

	// Target is up: answer locally.
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Authoritative = true

	switch q.Qtype {
	case dns.TypeA:
		rr := &dns.A{
			Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
			A:   parseIP4(entry.Target),
		}
		if rr.A != nil {
			resp.Answer = append(resp.Answer, rr)
		}
	case dns.TypeAAAA:
		// We only hand out the internal IPv4 target. Returning NODATA (empty,
		// success) stops the browser from chasing a nonexistent AAAA upstream.
	default:
		// Other qtypes for our names: forward so SRV/TXT/etc. still resolve.
		r.forward(w, req)
		return
	}
	_ = w.WriteMsg(resp)
}

// matchLocked finds the entry whose pattern covers name, unless name is one of
// that entry's exceptions (kept on public DNS). Caller holds r.mu. An excepted
// name skips to the next entry so an overlapping entry can still match.
func (r *Resolver) matchLocked(name string) (config.Entry, bool) {
	for _, e := range r.entries {
		if matchPattern(e.Pattern, name) {
			if matchesAnyException(e.Except, name) {
				continue
			}
			return e, true
		}
	}
	return config.Entry{}, false
}

// matchesAnyException reports whether name matches any exception pattern, reusing
// matchPattern so exact ("foo.corp.io") and wildcard ("*.dev.corp.io") exceptions
// both work.
func matchesAnyException(except []string, name string) bool {
	for _, ex := range except {
		if matchPattern(ex, name) {
			return true
		}
	}
	return false
}

// matchPattern matches a domain against a pattern. "*.example.com" matches any
// subdomain of example.com (and the apex example.com itself). A plain pattern
// matches exactly.
func matchPattern(pattern, name string) bool {
	pattern = strings.ToLower(strings.TrimSuffix(pattern, "."))
	if strings.HasPrefix(pattern, "*.") {
		base := pattern[2:]
		return name == base || strings.HasSuffix(name, "."+base)
	}
	return name == pattern
}
