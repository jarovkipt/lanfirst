#!/usr/bin/env bash
# lanfirst installer (macOS). Builds the daemon, menu-bar app, and the privileged
# resolver-sync helper; installs the user LaunchAgents plus one root LaunchDaemon.
# After this one-time install, domains are added/removed entirely from the menu bar
# and /etc/resolver stays in sync automatically — no recurring sudo. See docs/adr/0003.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
HOME_DIR="$HOME"
CFG_DIR="$HOME_DIR/.config/lanfirst"
CFG="$CFG_DIR/config.yaml"
LA_DIR="$HOME_DIR/Library/LaunchAgents"
BIN_DIR="$HOME_DIR/.local/bin"
APP_DIR="/Applications/lanfirst.app"
SBIN_DIR="/usr/local/sbin"                 # root-owned home for the privileged helper
LD_DIR="/Library/LaunchDaemons"            # root LaunchDaemon location

command -v go >/dev/null || { echo "Go toolchain required (brew install go)"; exit 1; }
[ "$(uname)" = "Darwin" ] || { echo "install.sh is for macOS"; exit 1; }

echo "==> Resolving dependencies"
( cd "$REPO_DIR" && go mod tidy )

echo "==> Building binaries"
mkdir -p "$BIN_DIR"
( cd "$REPO_DIR" && go build -o "$BIN_DIR/lanfirstd" ./cmd/lanfirstd )
# Staged build of the privileged helper; installed root-owned to $SBIN_DIR below.
( cd "$REPO_DIR" && go build -o "$BIN_DIR/lanfirst-resolverd" ./cmd/lanfirst-resolverd )

echo "==> Building menu-bar app bundle"
mkdir -p "$APP_DIR/Contents/MacOS"
( cd "$REPO_DIR" && go build -o "$APP_DIR/Contents/MacOS/lanfirst" ./cmd/lanfirst )
# Template menu-bar icons (committed PNGs from assets/render-icons.swift, with @2x
# Retina reps). menuet loads them by name via [NSImage imageNamed:] from the
# bundle's Resources dir and renders them as light/dark-aware template images.
mkdir -p "$APP_DIR/Contents/Resources"
cp "$REPO_DIR"/assets/icons/*.png "$APP_DIR/Contents/Resources/"
# App icon (assets/render-appicon.swift). The app is LSUIElement so it never
# shows in the Dock, but the icon appears in Finder, Get Info, and sharing UI.
cp "$REPO_DIR/assets/AppIcon.icns" "$APP_DIR/Contents/Resources/"
cat > "$APP_DIR/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>lanfirst</string>
  <key>CFBundleIdentifier</key><string>com.lanfirst.menubar</string>
  <key>CFBundleExecutable</key><string>lanfirst</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>LSUIElement</key><true/>
</dict></plist>
PLIST

echo "==> Installing config"
mkdir -p "$CFG_DIR"
if [ ! -f "$CFG" ]; then
  cp "$REPO_DIR/config.example.yaml" "$CFG"
  echo "    wrote $CFG (edit your entries here)"
else
  echo "    $CFG exists, leaving it"
fi

echo "==> Installing LaunchAgents"
mkdir -p "$LA_DIR"
sed -e "s#__BIN__#$BIN_DIR/lanfirstd#g" -e "s#__HOME__#$HOME_DIR#g" \
  "$REPO_DIR/install/com.lanfirst.daemon.plist" > "$LA_DIR/com.lanfirst.daemon.plist"
sed -e "s#__APP__#$APP_DIR#g" -e "s#__HOME__#$HOME_DIR#g" \
  "$REPO_DIR/install/com.lanfirst.menubar.plist" > "$LA_DIR/com.lanfirst.menubar.plist"

launchctl unload "$LA_DIR/com.lanfirst.daemon.plist" 2>/dev/null || true
launchctl load "$LA_DIR/com.lanfirst.daemon.plist"
launchctl unload "$LA_DIR/com.lanfirst.menubar.plist" 2>/dev/null || true
launchctl load "$LA_DIR/com.lanfirst.menubar.plist"

echo "==> Installing privileged resolver-sync helper (the only sudo)"
# A root LaunchDaemon must not exec a user-writable binary: install it root-owned and
# non-user-writable. The helper itself creates/removes /etc/resolver files from config.
sudo install -d -o root -g wheel -m 755 "$SBIN_DIR"
sudo install -o root -g wheel -m 755 "$BIN_DIR/lanfirst-resolverd" "$SBIN_DIR/lanfirst-resolverd"
rm -f "$BIN_DIR/lanfirst-resolverd"   # staged copy no longer needed

RESOLVERD_PLIST="$LD_DIR/com.lanfirst.resolverd.plist"
TMP_PLIST="$(mktemp)"
sed -e "s#__BIN__#$SBIN_DIR/lanfirst-resolverd#g" -e "s#__HOME__#$HOME_DIR#g" \
  "$REPO_DIR/install/com.lanfirst.resolverd.plist" > "$TMP_PLIST"
sudo install -o root -g wheel -m 644 "$TMP_PLIST" "$RESOLVERD_PLIST"
rm -f "$TMP_PLIST"

sudo launchctl bootout system "$RESOLVERD_PLIST" 2>/dev/null || true
sudo launchctl bootstrap system "$RESOLVERD_PLIST"
echo "    resolverd loaded; it reconciles /etc/resolver from your config automatically."

echo
echo "Done. Reminder: turn OFF Chrome 'Secure DNS' (chrome://settings/security)."
echo "Add/remove domains from the menu bar — no further sudo needed."
echo "Verify:  dig @127.0.0.1 -p 5354 app.example.com   (port = your config 'listen')"
