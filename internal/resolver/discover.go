package resolver

import (
	"bufio"
	"net"
	"os/exec"
	"strings"
)

// DiscoverUpstreams returns the explicit upstream resolvers to forward to.
//
// If configured is non-empty, those are used (normalised to host:port). Otherwise
// it parses `scutil --dns` for system nameservers, EXCLUDING our own listen
// address (selfHostPort) so we never forward back into ourselves. As a last
// resort it falls back to public resolvers.
func DiscoverUpstreams(selfHostPort string, configured []string) []string {
	if len(configured) > 0 {
		return normalise(configured)
	}

	selfHost, _, _ := net.SplitHostPort(selfHostPort)
	var found []string
	seen := map[string]struct{}{}

	out, err := exec.Command("scutil", "--dns").Output()
	if err == nil {
		sc := bufio.NewScanner(strings.NewReader(string(out)))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			// lines look like: "nameserver[0] : 1.1.1.1"
			if !strings.HasPrefix(line, "nameserver[") {
				continue
			}
			i := strings.LastIndex(line, ":")
			if i < 0 {
				continue
			}
			ip := strings.TrimSpace(line[i+1:])
			if ip == "" || ip == selfHost || net.ParseIP(ip) == nil {
				continue
			}
			if _, dup := seen[ip]; dup {
				continue
			}
			seen[ip] = struct{}{}
			found = append(found, net.JoinHostPort(ip, "53"))
		}
	}

	if len(found) == 0 {
		return []string{"1.1.1.1:53", "8.8.8.8:53"}
	}
	return found
}

func normalise(servers []string) []string {
	out := make([]string, 0, len(servers))
	for _, s := range servers {
		if _, _, err := net.SplitHostPort(s); err != nil {
			s = net.JoinHostPort(s, "53")
		}
		out = append(out, s)
	}
	return out
}
