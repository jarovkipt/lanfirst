package resolver

import (
	"testing"

	"github.com/jarovkipt/lanfirst/internal/config"
)

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"*.example.com", "example.com", true},        // apex
		{"*.example.com", "app.example.com", true},    // subdomain
		{"*.example.com", "a.b.example.com", true},    // deep subdomain
		{"*.example.com", "notexample.com", false},    // suffix trap
		{"*.example.com", "example.com.evil.com", false},
		{"app.example.internal", "app.example.internal", true}, // exact
		{"app.example.internal", "other.example.internal", false},
		{"*.example.com.", "app.example.com", true}, // trailing dot in pattern
	}
	for _, c := range cases {
		if got := matchPattern(c.pattern, c.name); got != c.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

// newResolver builds a Resolver with the given entries directly, bypassing the
// health checker (matchLocked does not consult it).
func newResolver(entries ...config.Entry) *Resolver {
	r := &Resolver{}
	r.entries = entries
	return r
}

func TestMatchLockedExceptions(t *testing.T) {
	wildcard := config.Entry{
		Pattern: "*.plshackme.com",
		Target:  "192.168.10.11",
		Port:    443,
		Except:  []string{"public.plshackme.com", "*.dev.plshackme.com"},
	}
	other := config.Entry{Pattern: "*.corp.io", Target: "10.0.0.5", Port: 443}
	r := newResolver(wildcard, other)

	cases := []struct {
		name        string
		wantMatch   bool
		wantPattern string
	}{
		{"app.plshackme.com", true, "*.plshackme.com"},  // matches, not excepted -> LAN
		{"public.plshackme.com", false, ""},             // exact exception -> forwarded
		{"api.dev.plshackme.com", false, ""},            // wildcard exception -> forwarded
		{"dev.plshackme.com", false, ""},                // apex of "*.dev..." exception -> forwarded
		{"anything.corp.io", true, "*.corp.io"},         // exception on other entry unaffected
	}
	for _, c := range cases {
		got, ok := r.matchLocked(c.name)
		if ok != c.wantMatch {
			t.Errorf("matchLocked(%q) matched=%v, want %v", c.name, ok, c.wantMatch)
			continue
		}
		if ok && got.Pattern != c.wantPattern {
			t.Errorf("matchLocked(%q) pattern=%q, want %q", c.name, got.Pattern, c.wantPattern)
		}
	}
}
