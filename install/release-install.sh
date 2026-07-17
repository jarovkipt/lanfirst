#!/usr/bin/env bash
# lanfirst release installer/updater. Ships inside the release tarball as
# `install.sh` next to prebuilt binaries — unlike install/install.sh it never
# compiles anything, so the target machine does not need Go.
#
# Two modes:
#   ./install.sh          terminal install/update; privileged step uses sudo
#   ./install.sh --gui    spawned by the menu-bar app's updater; privileged step
#                         uses osascript so the user gets the native macOS
#                         admin-password dialog, and completion is announced
#                         with a notification instead of stdout
#
# The privileged step runs FIRST and is a single escalation: cancelling the
# password prompt aborts before anything on the machine has changed.
set -euo pipefail

STAGE="$(cd "$(dirname "$0")" && pwd)"   # extracted lanfirst-vX.Y.Z dir
VERSION="${STAGE##*/lanfirst-}"          # "v0.1.0" (best-effort, cosmetic only)

GUI=0
if [ "${1:-}" = "--gui" ]; then GUI=1; fi

HOME_DIR="$HOME"
CFG_DIR="$HOME_DIR/.config/lanfirst"
CFG="$CFG_DIR/config.yaml"
LA_DIR="$HOME_DIR/Library/LaunchAgents"
BIN_DIR="$HOME_DIR/.local/bin"
APP_DIR="/Applications/lanfirst.app"
SBIN_DIR="/usr/local/sbin"
LD_DIR="/Library/LaunchDaemons"
UID_NUM="$(id -u)"

fail() {
  echo "ERROR: $1" >&2
  if [ "$GUI" = 1 ]; then
    osascript -e "display notification \"$1\" with title \"lanfirst update failed\"" || true
  fi
  exit 1
}
trap 'fail "install step failed (see ~/Library/Logs/lanfirst-update.log)"' ERR

[ "$(uname)" = "Darwin" ] || fail "this installer is for macOS"
[ -x "$STAGE/bin/lanfirstd" ] || fail "tarball incomplete: bin/lanfirstd missing"
[ -x "$STAGE/bin/lanfirst-resolverd" ] || fail "tarball incomplete: bin/lanfirst-resolverd missing"
[ -d "$STAGE/lanfirst.app" ] || fail "tarball incomplete: lanfirst.app missing"

echo "==> Installing lanfirst $VERSION from $STAGE"

# --- 1. Privileged part first: resolver-sync helper + its root LaunchDaemon.
# Template the plist unprivileged, then run one root script (single prompt).
RESOLVERD_PLIST_TMP="$(mktemp /tmp/lanfirst-resolverd-plist.XXXXXX)"
sed -e "s#__BIN__#$SBIN_DIR/lanfirst-resolverd#g" -e "s#__HOME__#$HOME_DIR#g" \
  "$STAGE/com.lanfirst.resolverd.plist" > "$RESOLVERD_PLIST_TMP"

PRIV_SCRIPT="$(mktemp /tmp/lanfirst-priv.XXXXXX)"
chmod 700 "$PRIV_SCRIPT"
cat > "$PRIV_SCRIPT" <<EOF
#!/bin/sh
set -e
install -d -o root -g wheel -m 755 "$SBIN_DIR"
install -o root -g wheel -m 755 "$STAGE/bin/lanfirst-resolverd" "$SBIN_DIR/lanfirst-resolverd"
install -o root -g wheel -m 644 "$RESOLVERD_PLIST_TMP" "$LD_DIR/com.lanfirst.resolverd.plist"
launchctl bootout system "$LD_DIR/com.lanfirst.resolverd.plist" 2>/dev/null || true
launchctl bootstrap system "$LD_DIR/com.lanfirst.resolverd.plist"
EOF

echo "==> Installing privileged resolver-sync helper (admin password required)"
if [ "$GUI" = 1 ]; then
  osascript -e "do shell script \"/bin/sh $PRIV_SCRIPT\" with administrator privileges" \
    || fail "update cancelled (admin authorization declined); nothing was changed"
else
  sudo /bin/sh "$PRIV_SCRIPT"
fi
rm -f "$PRIV_SCRIPT" "$RESOLVERD_PLIST_TMP"

# --- 2. User daemon binary.
mkdir -p "$BIN_DIR"
install -m 755 "$STAGE/bin/lanfirstd" "$BIN_DIR/lanfirstd"

# --- 3. App bundle swap, with rollback if the copy fails midway.
rm -rf "$APP_DIR.old"
if [ -d "$APP_DIR" ]; then mv "$APP_DIR" "$APP_DIR.old"; fi
if ! cp -R "$STAGE/lanfirst.app" "$APP_DIR"; then
  rm -rf "$APP_DIR"
  if [ -d "$APP_DIR.old" ]; then mv "$APP_DIR.old" "$APP_DIR"; fi
  fail "could not install app bundle; previous version restored"
fi
# In-app and curl downloads never carry quarantine, but a browser-downloaded
# tarball would — strip it so Gatekeeper doesn't block the unsigned bundle.
xattr -dr com.apple.quarantine "$APP_DIR" 2>/dev/null || true

# --- 4. Config + LaunchAgents (idempotent; plists are cheap to regenerate).
mkdir -p "$CFG_DIR"
if [ ! -f "$CFG" ]; then
  cp "$STAGE/config.example.yaml" "$CFG"
  echo "    wrote $CFG (edit your entries here)"
fi
mkdir -p "$LA_DIR"
sed -e "s#__BIN__#$BIN_DIR/lanfirstd#g" -e "s#__HOME__#$HOME_DIR#g" \
  "$STAGE/com.lanfirst.daemon.plist" > "$LA_DIR/com.lanfirst.daemon.plist"
sed -e "s#__APP__#$APP_DIR#g" -e "s#__HOME__#$HOME_DIR#g" \
  "$STAGE/com.lanfirst.menubar.plist" > "$LA_DIR/com.lanfirst.menubar.plist"
# --- 5. Re-register both user agents onto the new binaries. We bootout then
# bootstrap (rather than bootstrap-if-absent + kickstart) so launchd re-pins its
# per-service launch constraint (LWCR) to the NEW binary's code-signature hash.
# On an in-place update the service is already loaded, so a plain bootstrap is a
# no-op and a following kickstart re-execs the swapped binary against the OLD
# cdhash — which macOS kills with SIGKILL "Code Signature Invalid". Both agents
# have RunAtLoad=true, so bootstrap starts them fresh; no kickstart needed. The
# updater is spawned detached (setsid), so booting out the menu-bar app that
# launched it does not kill this script. (The root resolverd above already does
# this same bootout/bootstrap dance for the same reason.)
for svc in com.lanfirst.daemon com.lanfirst.menubar; do
  launchctl bootout "gui/$UID_NUM/$svc" 2>/dev/null || true
  launchctl bootstrap "gui/$UID_NUM" "$LA_DIR/$svc.plist"
done

# --- 6. Done.
rm -rf "$APP_DIR.old"
if [ "$GUI" = 1 ]; then
  osascript -e "display notification \"Updated to $VERSION\" with title \"lanfirst\"" || true
else
  echo
  echo "Done. lanfirst $VERSION installed."
  echo "Reminder: turn OFF Chrome 'Secure DNS' (chrome://settings/security)."
  echo "Verify:  dig @127.0.0.1 -p 5354 app.example.com   (port = your config 'listen')"
fi
