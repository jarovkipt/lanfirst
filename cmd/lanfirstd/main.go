// Command lanfirstd is the always-on lanfirst daemon: a split-horizon DNS
// resolver with reachability-based failover, plus a Unix-socket control API.
//
// It runs as a macOS LaunchAgent (KeepAlive=true) so that resolution survives
// quitting or crashing the menu-bar app.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/jarovkipt/lanfirst/internal/config"
	"github.com/jarovkipt/lanfirst/internal/health"
	"github.com/jarovkipt/lanfirst/internal/ipc"
	"github.com/jarovkipt/lanfirst/internal/resolver"
	"github.com/jarovkipt/lanfirst/internal/version"
	"github.com/miekg/dns"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "path to config.yaml")
	showVersion := flag.Bool("version", false, "print build version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("lanfirstd", version.String())
		return
	}
	log.Printf("lanfirstd starting, version %s", version.String())

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	checker := health.New(cfg.Health.Timeout)
	checker.SetTargets(targetsOf(cfg))

	upstream := resolver.DiscoverUpstreams(cfg.Listen, cfg.Upstream)
	res := resolver.New(cfg, checker, upstream)

	stop := make(chan struct{})

	// Health-check loop.
	go checker.Run(cfg.Health.Interval, stop)

	// DNS servers (UDP + TCP) on the configured listen address.
	mux := dns.NewServeMux()
	mux.Handle(".", res)
	udp := &dns.Server{Addr: cfg.Listen, Net: "udp", Handler: mux}
	tcp := &dns.Server{Addr: cfg.Listen, Net: "tcp", Handler: mux}
	go mustServe(udp)
	go mustServe(tcp)
	log.Printf("lanfirstd listening on %s, upstream=%v", cfg.Listen, upstream)

	// Config watcher: reload entries/targets/upstreams on file change.
	var reloadMu sync.Mutex
	reload := func() {
		reloadMu.Lock()
		defer reloadMu.Unlock()
		nc, err := config.Load(*cfgPath)
		if err != nil {
			log.Printf("reload skipped: %v", err)
			return
		}
		checker.SetTargets(targetsOf(nc))
		res.Update(nc, resolver.DiscoverUpstreams(nc.Listen, nc.Upstream))
		log.Printf("config reloaded: %d entries", len(nc.Entries))
	}
	go func() {
		if err := config.Watch(*cfgPath, reload, stop); err != nil {
			log.Printf("config watch stopped: %v", err)
		}
	}()

	// mutateConfig applies fn to a freshly-loaded config, persists it atomically,
	// and reloads routing — all under reloadMu so the daemon is the single writer.
	// fn reports whether it changed anything (false → no write, no reload).
	mutateConfig := func(fn func(*config.Config) (bool, error)) error {
		reloadMu.Lock()
		defer reloadMu.Unlock()
		c, err := config.Load(*cfgPath)
		if err != nil {
			return err
		}
		changed, err := fn(c)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		if err := config.Save(*cfgPath, c); err != nil {
			return err
		}
		checker.SetTargets(targetsOf(c))
		res.Update(c, resolver.DiscoverUpstreams(c.Listen, c.Upstream))
		log.Printf("config mutated: %d entries", len(c.Entries))
		return nil
	}

	// Control socket for the menu-bar app.
	go func() {
		err := ipc.Serve(ipc.SocketPath(), func(req ipc.Request) ipc.Response {
			switch req.Command {
			case ipc.CmdEnable:
				res.SetEnabled(true)
			case ipc.CmdDisable:
				res.SetEnabled(false)
			case ipc.CmdReload:
				reload()
			case ipc.CmdAddEntry:
				if err := mutateConfig(func(c *config.Config) (bool, error) {
					return true, c.AddEntry(config.Entry{Pattern: req.Pattern, Target: req.Target, Port: req.Port})
				}); err != nil {
					return ipc.Response{OK: false, Error: err.Error()}
				}
			case ipc.CmdRemoveEntry:
				if err := mutateConfig(func(c *config.Config) (bool, error) {
					if !c.RemoveEntry(req.Pattern) {
						return false, fmt.Errorf("no entry with pattern %q", req.Pattern)
					}
					return true, nil
				}); err != nil {
					return ipc.Response{OK: false, Error: err.Error()}
				}
			case ipc.CmdStatus:
				// fallthrough to status below
			default:
				return ipc.Response{OK: false, Error: "unknown command"}
			}
			return status(res)
		}, stop)
		if err != nil {
			log.Printf("ipc server stopped: %v", err)
		}
	}()

	// Wait for termination.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	close(stop)
	_ = udp.Shutdown()
	_ = tcp.Shutdown()
}

func status(res *resolver.Resolver) ipc.Response {
	modes := res.Modes()
	entries := make([]ipc.EntryStatus, 0, len(modes))
	for _, m := range modes {
		entries = append(entries, ipc.EntryStatus{Pattern: m.Pattern, Target: m.Target, LAN: m.LAN})
	}
	return ipc.Response{OK: true, Enabled: res.Enabled(), Version: version.String(), Entries: entries}
}

func targetsOf(c *config.Config) []health.Target {
	seen := map[health.Target]struct{}{}
	var out []health.Target
	for _, e := range c.Entries {
		t := health.Target{IP: e.Target, Port: e.Port}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func mustServe(s *dns.Server) {
	if err := s.ListenAndServe(); err != nil {
		log.Fatalf("dns %s server: %v", s.Net, err)
	}
}
