// Command lanfirst is the menu-bar controller for the lanfirst daemon. It is a
// thin client: it shows per-entry routing state, lets you Add/Remove domains via
// dialogs, and sends enable/disable/reload commands over the Unix control socket.
// Quitting it does NOT stop DNS routing — the daemon (lanfirstd) keeps running.
package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/caseymrm/menuet"
	"github.com/jarovkipt/lanfirst/internal/ipc"
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
