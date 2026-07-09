package resolver

import (
	"net"
	"time"

	"github.com/miekg/dns"
)

// parseIP4 returns the 4-byte form of an IPv4 address string, or nil.
func parseIP4(s string) net.IP {
	ip := net.ParseIP(s)
	if ip == nil {
		return nil
	}
	return ip.To4()
}

// forward proxies the query to the configured explicit upstreams and writes the
// first successful answer back. It deliberately uses explicit DNS servers and
// never the system resolver, otherwise /etc/resolver would route the query
// straight back to this daemon and loop forever.
//
// Forwarded answers carry the upstream's TTL, which can be minutes. We clamp it
// down to our own ttl so the OS resolver cache releases public-mode answers as
// quickly as LAN-mode ones — otherwise toggling routing back on stalls until the
// upstream TTL expires.
func (r *Resolver) forward(w dns.ResponseWriter, req *dns.Msg) {
	r.mu.RLock()
	ups := append([]string(nil), r.upstream...)
	ttl := r.ttl
	r.mu.RUnlock()

	c := &dns.Client{Timeout: 3 * time.Second}
	for _, up := range ups {
		resp, _, err := c.Exchange(req, up)
		if err == nil && resp != nil {
			clampTTL(resp, ttl)
			_ = w.WriteMsg(resp)
			return
		}
	}
	// All upstreams failed: return SERVFAIL rather than hanging.
	fail := new(dns.Msg)
	fail.SetRcode(req, dns.RcodeServerFailure)
	_ = w.WriteMsg(fail)
}

// clampTTL lowers the TTL of every record in msg to at most max, so a forwarded
// answer is cached no longer than our own internal answers. OPT (EDNS) records
// are skipped: their header TTL field encodes extended rcode/version/flags, not
// a cache lifetime, so rewriting it would corrupt EDNS.
func clampTTL(msg *dns.Msg, max uint32) {
	for _, section := range [][]dns.RR{msg.Answer, msg.Ns, msg.Extra} {
		for _, rr := range section {
			if rr.Header().Rrtype == dns.TypeOPT {
				continue
			}
			if h := rr.Header(); h.Ttl > max {
				h.Ttl = max
			}
		}
	}
}
