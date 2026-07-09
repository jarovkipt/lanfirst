// Command lanfirst is the menu-bar controller for the lanfirst daemon. It is a
// thin client: it shows per-entry routing state, lets you Add/Remove domains via
// dialogs, and sends enable/disable/reload commands over the Unix control socket.
// Quitting it does NOT stop DNS routing — the daemon (lanfirstd) keeps running.
package main

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/caseymrm/menuet"
	"github.com/jarovkipt/lanfirst/internal/ipc"
	"github.com/jarovkipt/lanfirst/internal/update"
	"github.com/jarovkipt/lanfirst/internal/version"
)

func currentStatus() (ipc.Response, error) {
	return ipc.Call(ipc.SocketPath(), ipc.CmdStatus)
}

// iconName maps the daemon's routing state to a template icon shipped in the
// app bundle's Resources dir (see assets/render-icons.swift). menuet renders it
// as a monochrome NSStatusItem template image that follows light/dark mode.
func iconName(resp ipc.Response, err error) string {
	if err != nil {
		return "lan-error" // daemon not reachable
	}
	if !resp.Enabled {
		return "lan-off" // routing disabled
	}
	for _, e := range resp.Entries {
		if e.LAN {
			return "lan-on" // at least one entry routing to LAN
		}
	}
	return "lan-public" // all entries on public DNS
}

func menuItems() []menuet.MenuItem {
	resp, err := currentStatus()
	if err != nil {
		return []menuet.MenuItem{
			{Text: "Daemon not reachable"},
			{Text: fmt.Sprintf("(%v)", err)},
		}
	}

	items := []menuet.MenuItem{
		{Text: fmt.Sprintf("LAN routing: %s", onOff(resp.Enabled))},
		{Type: menuet.Separator},
	}
	for _, e := range resp.Entries {
		e := e // capture per entry for the submenu closure
		mark := "⚪ public"
		if e.LAN {
			mark = "🟢 LAN → " + e.Target
		}
		items = append(items, menuet.MenuItem{
			Text:     fmt.Sprintf("%s  —  %s", e.Pattern, mark),
			Children: entrySubmenu(e),
		})
	}
	items = append(items,
		menuet.MenuItem{Type: menuet.Separator},
		menuet.MenuItem{Text: "Add domain…", Clicked: addDomain},
		menuet.MenuItem{Type: menuet.Separator},
		menuet.MenuItem{
			Text:    toggleLabel(resp.Enabled),
			Clicked: func() { toggle(resp.Enabled) },
		},
		menuet.MenuItem{Text: "Reload config", Clicked: func() { _, _ = ipc.Call(ipc.SocketPath(), ipc.CmdReload) }},
		menuet.MenuItem{Type: menuet.Separator},
		menuet.MenuItem{Text: "App: " + version.String()},
		menuet.MenuItem{Text: "Daemon: " + daemonVersion(resp)},
	)
	if r := latestUpdate(); r != nil {
		items = append(items, menuet.MenuItem{
			Text:    "Update to " + r.Tag + "…",
			Clicked: func() { applyUpdate(r) },
		})
	}
	items = append(items, menuet.MenuItem{Text: "Check for Updates…", Clicked: manualUpdateCheck})
	return items
}

// daemonVersion reports the running daemon's build identity from a status
// response, or a placeholder for an older daemon that predates version reporting.
func daemonVersion(resp ipc.Response) string {
	if resp.Version == "" {
		return "unknown (restart daemon)"
	}
	return resp.Version
}

// entrySubmenu returns the children for one resolver entry: its target/mode and a
// Remove action.
func entrySubmenu(e ipc.EntryStatus) func() []menuet.MenuItem {
	return func() []menuet.MenuItem {
		mode := "public DNS"
		if e.LAN {
			mode = "LAN → " + e.Target
		}
		return []menuet.MenuItem{
			{Text: "Target: " + e.Target},
			{Text: "Mode: " + mode},
			{Type: menuet.Separator},
			{Text: "Remove…", Clicked: func() { removeDomain(e.Pattern) }},
		}
	}
}

// addDomain prompts for a new resolver entry and sends it to the daemon.
func addDomain() {
	res := menuet.App().Alert(menuet.Alert{
		MessageText:     "Add domain",
		InformativeText: "Route a domain to an internal target while that target is reachable.",
		Buttons:         []string{"Add", "Cancel"},
		Inputs:          []string{"pattern e.g. *.corp.io", "target IP e.g. 192.168.1.10", "port (default 443)"},
	})
	if res.Button != 0 || len(res.Inputs) < 3 { // 0 == "Add"
		return
	}
	pattern := strings.TrimSpace(res.Inputs[0])
	target := strings.TrimSpace(res.Inputs[1])
	portStr := strings.TrimSpace(res.Inputs[2])
	if pattern == "" || target == "" {
		notifyErr("Pattern and target are required.")
		return
	}
	port := 443
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil || p <= 0 || p > 65535 {
			notifyErr(fmt.Sprintf("Invalid port %q.", portStr))
			return
		}
		port = p
	}
	resp, err := ipc.CallRequest(ipc.SocketPath(), ipc.Request{
		Command: ipc.CmdAddEntry, Pattern: pattern, Target: target, Port: port,
	})
	if err != nil {
		notifyErr(fmt.Sprintf("Daemon unreachable: %v", err))
		return
	}
	if !resp.OK {
		notifyErr(resp.Error)
	}
}

// removeDomain confirms and removes the entry with the given pattern.
func removeDomain(pattern string) {
	res := menuet.App().Alert(menuet.Alert{
		MessageText:     "Remove " + pattern + "?",
		InformativeText: "Its /etc/resolver entry is removed automatically.",
		Buttons:         []string{"Remove", "Cancel"},
	})
	if res.Button != 0 { // 0 == "Remove"
		return
	}
	resp, err := ipc.CallRequest(ipc.SocketPath(), ipc.Request{
		Command: ipc.CmdRemoveEntry, Pattern: pattern,
	})
	if err != nil {
		notifyErr(fmt.Sprintf("Daemon unreachable: %v", err))
		return
	}
	if !resp.OK {
		notifyErr(resp.Error)
	}
}

// updates holds the newest release found by the background check, if it is
// newer than this build. menuItems reads it on every menu open.
var updates struct {
	mu     sync.Mutex
	latest *update.Release
}

func latestUpdate() *update.Release {
	updates.mu.Lock()
	defer updates.mu.Unlock()
	return updates.latest
}

func setLatestUpdate(r *update.Release) {
	updates.mu.Lock()
	defer updates.mu.Unlock()
	updates.latest = r
}

// backgroundUpdateCheck polls GitHub Releases once a day. Failures are silent
// (offline is normal); dev builds (no release tag) are never nagged.
func backgroundUpdateCheck() {
	time.Sleep(15 * time.Second) // let the network come up after login
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		r, err := update.Check(ctx)
		cancel()
		if err == nil && update.IsNewer(version.ReleaseTag(), r) {
			setLatestUpdate(r)
		}
		time.Sleep(24 * time.Hour)
	}
}

// manualUpdateCheck is the "Check for Updates…" click handler (menuet runs it
// in its own goroutine, so blocking on the network here is fine).
func manualUpdateCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := update.Check(ctx)
	if err != nil {
		notifyErr(fmt.Sprintf("Update check failed: %v", err))
		return
	}
	cur := version.ReleaseTag()
	switch {
	case cur == "":
		res := menuet.App().Alert(menuet.Alert{
			MessageText:     "lanfirst",
			InformativeText: fmt.Sprintf("You're running a dev build (%s).\nLatest release is %s.", version.String(), r.Tag),
			Buttons:         []string{"Install " + r.Tag, "Cancel"},
		})
		if res.Button == 0 {
			downloadAndInstall(r)
		}
	case update.IsNewer(cur, r):
		setLatestUpdate(r)
		applyUpdate(r)
	default:
		menuet.App().Alert(menuet.Alert{
			MessageText:     "lanfirst",
			InformativeText: fmt.Sprintf("You're up to date (%s).", cur),
			Buttons:         []string{"OK"},
		})
	}
}

// applyUpdate confirms and installs the given release. The bundled installer
// script asks for the admin password (root helper) and restarts this app via
// launchctl, so on success this process simply gets killed and relaunched.
func applyUpdate(r *update.Release) {
	res := menuet.App().Alert(menuet.Alert{
		MessageText:     "Update lanfirst to " + r.Tag + "?",
		InformativeText: "You'll be asked for your administrator password, and the menu bar app will restart.",
		Buttons:         []string{"Update", "Cancel"},
	})
	if res.Button != 0 {
		return
	}
	downloadAndInstall(r)
}

func downloadAndInstall(r *update.Release) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stage, err := update.DownloadAndVerify(ctx, r)
	if err != nil {
		notifyErr(fmt.Sprintf("Update failed, current version untouched: %v", err))
		return
	}
	if err := update.LaunchInstaller(stage); err != nil {
		notifyErr(fmt.Sprintf("Could not start installer: %v", err))
	}
	// The installer takes it from here: admin prompt, file swap, kickstart.
}

func notifyErr(msg string) {
	menuet.App().Alert(menuet.Alert{
		MessageText:     "lanfirst",
		InformativeText: msg,
		Buttons:         []string{"OK"},
	})
}

func toggle(enabled bool) {
	cmd := ipc.CmdEnable
	if enabled {
		cmd = ipc.CmdDisable
	}
	_, _ = ipc.Call(ipc.SocketPath(), cmd)
}

func toggleLabel(enabled bool) string {
	if enabled {
		return "Disable LAN routing"
	}
	return "Enable LAN routing"
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func main() {
	showVersion := flag.Bool("version", false, "print build version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("lanfirst", version.String())
		return
	}

	app := menuet.App()
	app.Name = "lanfirst"
	app.Label = "com.lanfirst.menubar"
	app.Children = menuItems

	go backgroundUpdateCheck()

	// Refresh the menu-bar title on a short cadence so it tracks failover.
	go func() {
		for {
			resp, err := currentStatus()
			app.SetMenuState(&menuet.MenuState{Image: iconName(resp, err)})
			time.Sleep(3 * time.Second)
		}
	}()

	app.RunApplication()
}
