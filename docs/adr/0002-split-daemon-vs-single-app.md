# Split always-on daemon vs single menu-bar app

lanfirst ships as two processes: an always-on daemon (`lanfirstd`, a LaunchAgent with
`KeepAlive=true`) that owns the DNS resolver and health-checker, and a menu-bar app
(`lanfirst`) that is a thin controller talking to the daemon over a Unix domain socket.

We rejected packaging everything in a single menu-bar `.app`. A scoped
`/etc/resolver/<domain>` entry sends those domains to `127.0.0.1:5354` and does **not**
fall back to the default system resolver if nothing is listening. So if a single app
were quit or crashed, the configured domains would fail to resolve entirely — on and
off VPN — which is worse than not having the tool and contradicts the goal of software
that "just runs".

Consequences: a local IPC protocol is needed between the two processes (line-delimited
JSON over a Unix socket). Quitting the menu-bar app leaves resolution intact; the
daemon is managed by launchd independently.
