// Package config loads and watches the lanfirst daemon configuration.
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// Entry is a single resolver entry: a match pattern routed to an internal target.
type Entry struct {
	Pattern string `yaml:"pattern"` // e.g. "*.example.com" or "app.example.internal"
	Target  string `yaml:"target"`  // internal IP of the reverse proxy, e.g. "192.168.1.10"
	Port    int    `yaml:"port"`    // TCP port used for the health-check, default 443
}

// Health controls the reachability probe cadence.
type Health struct {
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
}

// Config is the full daemon configuration.
type Config struct {
	Listen   string   `yaml:"listen"`   // "127.0.0.1:5354"
	Health   Health   `yaml:"health"`   //
	TTL      uint32   `yaml:"ttl"`      // DNS TTL (seconds) for internal answers
	Upstream []string `yaml:"upstream"` // explicit fallback DNS servers; empty = auto from scutil --dns
	Entries  []Entry  `yaml:"entries"`
}

// DefaultPath returns ~/.config/lanfirst/config.yaml.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "lanfirst", "config.yaml")
}

// Load reads and validates the config file, applying sensible defaults.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = "127.0.0.1:5354"
	}
	if c.Health.Interval == 0 {
		c.Health.Interval = 5 * time.Second
	}
	if c.Health.Timeout == 0 {
		c.Health.Timeout = time.Second
	}
	if c.TTL == 0 {
		// Low TTL keeps the OS resolver cache short-lived so toggling routing
		// (and reachability failover) is reflected quickly. It also caps the TTL
		// of forwarded public-mode answers (see resolver.clampTTL).
		c.TTL = 1
	}
	for i := range c.Entries {
		if c.Entries[i].Port == 0 {
			c.Entries[i].Port = 443
		}
	}
}

func (c *Config) validate() error {
	for _, e := range c.Entries {
		if e.Pattern == "" {
			return fmt.Errorf("entry with empty pattern")
		}
		if e.Target == "" {
			return fmt.Errorf("entry %q has empty target", e.Pattern)
		}
	}
	return nil
}

// ResolverDomains returns the distinct parent domains to route to lanfirst, one
// per /etc/resolver file: each entry's pattern with a leading "*." stripped,
// lowercased, deduped and sorted. Mirrors the matching in resolver.matchPattern.
func (c *Config) ResolverDomains() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, e := range c.Entries {
		d := strings.ToLower(strings.TrimSuffix(e.Pattern, "."))
		d = strings.TrimPrefix(d, "*.")
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// ListenPort parses the port from the Listen address (e.g. "127.0.0.1:5354").
func (c *Config) ListenPort() (int, error) {
	_, portStr, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return 0, fmt.Errorf("parse listen %q: %w", c.Listen, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("parse listen port %q: %w", portStr, err)
	}
	return port, nil
}

// AddEntry appends a resolver entry after applying defaults and validating it.
// It rejects a pattern that already exists (dedupe by pattern).
func (c *Config) AddEntry(e Entry) error {
	if e.Port == 0 {
		e.Port = 443
	}
	if e.Pattern == "" {
		return fmt.Errorf("entry with empty pattern")
	}
	if e.Target == "" {
		return fmt.Errorf("entry %q has empty target", e.Pattern)
	}
	for _, ex := range c.Entries {
		if ex.Pattern == e.Pattern {
			return fmt.Errorf("entry %q already exists", e.Pattern)
		}
	}
	c.Entries = append(c.Entries, e)
	return nil
}

// RemoveEntry drops the entry with the given pattern, returning whether one was
// removed.
func (c *Config) RemoveEntry(pattern string) bool {
	for i, e := range c.Entries {
		if e.Pattern == pattern {
			c.Entries = append(c.Entries[:i], c.Entries[i+1:]...)
			return true
		}
	}
	return false
}

// Save atomically writes the config to path (temp file in the same dir + rename).
// NB: this serialises via yaml.Marshal and so does not preserve comments — the
// config is app-managed; the annotated template lives in config.example.yaml.
func Save(path string, c *Config) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp into place: %w", err)
	}
	return nil
}

// Watch calls onChange whenever the config file is written. It debounces rapid
// editor writes. Blocks until ctx-equivalent stop channel is closed.
func Watch(path string, onChange func(), stop <-chan struct{}) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	// Watch the parent dir: editors often replace the file (rename), which
	// drops a watch placed directly on the file.
	if err := w.Add(filepath.Dir(path)); err != nil {
		return err
	}

	var timer *time.Timer
	for {
		select {
		case <-stop:
			return nil
		case ev := <-w.Events:
			if filepath.Clean(ev.Name) != filepath.Clean(path) {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(200*time.Millisecond, onChange)
		case err := <-w.Errors:
			if err != nil {
				return err
			}
		}
	}
}
