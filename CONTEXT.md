# lanfirst

A macOS resolver that sends configured domains to an internal reverse proxy when
that proxy is reachable, and otherwise lets them resolve over public DNS — so the
same web apps "just work" whether or not you are on the VPN.

## Language

**Resolver entry**:
One rule pairing a match pattern with an internal target. The unit a user adds to
route a domain.
_Avoid_: rule, mapping, route

**Match pattern**:
The domain name or wildcard an entry covers, e.g. `*.example.com` or
`app.example.internal`. A wildcard covers the apex and all subdomains.
_Avoid_: glob, host filter

**Internal target**:
The internal IP (a reverse proxy) an entry points to when that proxy is reachable.
One target may serve several patterns; different patterns may have different targets.
_Avoid_: backend, origin, server

**Reachability**:
Whether the internal target answers a TCP connection on its port right now. The
single signal that decides an entry's mode. Determined by the health-check and read
from cache.
_Avoid_: liveness, ping, availability

**Health-check**:
The periodic TCP probe of each distinct internal target that produces reachability.
_Avoid_: heartbeat, monitor

**LAN mode**:
An entry's state when its target is reachable and routing is enabled: queries are
answered with the internal target.
_Avoid_: internal mode, direct mode

**Public mode**:
An entry's state when its target is unreachable (or routing is disabled): queries
are forwarded to public DNS.
_Avoid_: external mode, fallback mode

**Fallback**:
The act of forwarding a query to public DNS because the internal target is
unreachable. The transition into public mode.
_Avoid_: failover (reserve for the overall behaviour, not this single act)

**Exception**:
A host carved out of an entry's match pattern that stays on public DNS even when
the target is reachable — e.g. keep `dl.corp.io` public under `*.corp.io`. Written
as a full host or wildcard; a bare subdomain (`dl`) is qualified against the
pattern's domain. Enforced in the resolver, so an excepted name simply falls back
to upstream.
_Avoid_: exclude, ignore, blocklist

**Upstream**:
The explicit public DNS servers lanfirst forwards to in public mode. Must never be
the system resolver, or queries loop back into lanfirst.
_Avoid_: forwarder, parent

## Example dialogue

> **Dev:** When I'm on the VPN, `grafana.example.com` should hit the internal box.
> Is that one entry or two?
>
> **Domain expert:** One resolver entry. Its match pattern is `*.example.com`, so it
> covers `grafana.` and everything else under it. The internal target is the reverse
> proxy at `192.168.1.10`.
>
> **Dev:** And off the VPN?
>
> **Domain expert:** The health-check can't open a TCP connection to the target, so
> reachability is false — the entry is in public mode. The query falls back to an
> upstream resolver, which returns the public IP.
>
> **Dev:** What if I also want `app.example.internal` pointed somewhere else?
>
> **Domain expert:** A second resolver entry, different internal target. Each target is
> health-checked independently, so one can be in LAN mode while the other is in public
> mode.
