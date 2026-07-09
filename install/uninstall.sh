#!/usr/bin/env bash
# Removes lanfirst: LaunchAgents, binaries, app bundle, and /etc/resolver entries.
# Leaves your ~/.config/lanfirst/config.yaml in place.
set -euo pipefail

HOME_DIR="$HOME"
CFG="$HOME_DIR/.config/lanfirst/config.yaml"
LA_DIR="$HOME_DIR/Library/LaunchAgents"
SBIN_DIR="/usr/local/sbin"
LD_DIR="/Library/LaunchDaemons"
RESOLVERD_PLIST="$LD_DIR/com.lanfirst.resolverd.plist"
RESOLVERD_BIN="$SBIN_DIR/lanfirst-resolverd"

echo "==> Unloading LaunchAgents"
launchctl unload "$LA_DIR/com.lanfirst.menubar.plist" 2>/dev/null || true
launchctl unload "$LA_DIR/com.lanfirst.daemon.plist" 2>/dev/null || true
rm -f "$LA_DIR/com.lanfirst.menubar.plist" "$LA_DIR/com.lanfirst.daemon.plist"

echo "==> Removing managed /etc/resolver entries + privileged helper (needs sudo)"
# Let the helper remove only the resolver files it created (marker-gated), then
# unload and remove the root LaunchDaemon and its binary.
if [ -x "$RESOLVERD_BIN" ]; then
  sudo "$RESOLVERD_BIN" -cleanup || true
fi
sudo launchctl bootout system "$RESOLVERD_PLIST" 2>/dev/null || true
sudo rm -f "$RESOLVERD_PLIST" "$RESOLVERD_BIN"

echo "==> Removing user binaries and app bundle"
rm -f "$HOME_DIR/.local/bin/lanfirstd"
rm -rf "/Applications/lanfirst.app"

echo "Done. (config left at $CFG)"
