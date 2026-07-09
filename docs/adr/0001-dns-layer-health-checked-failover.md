# DNS-layer health-checked failover

We resolve internal-vs-public per domain at the DNS layer: a local resolver answers
configured patterns with the internal target when a TCP health-check to that target
succeeds, and forwards to public DNS otherwise.

We rejected a **PAC file + forward proxy** approach: the infrastructure exposes
*reverse* proxies (one IP serving many hostnames by Host header with valid TLS), not
forward proxies, so there is nothing for a PAC `PROXY` directive to point at, and we
would have to stand up a proxy per target.

We rejected **dnsmasq with a config-toggling script**: dnsmasq cannot health-check, so
we would script reachability and flip `address=` lines, but per-target health across
many wildcard zones plus a process restart on every transition (which dumps the cache)
is fragile compared to a resolver that reads cached health per query.

Consequences: macOS `/etc/resolver/<domain>` routes only the system resolver, so
browsers with DoH ("Secure DNS") bypass lanfirst — Chrome must have Secure DNS off.
DNS/browser caching bounds switch latency to a few seconds even with a short TTL.
