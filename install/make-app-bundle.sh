#!/usr/bin/env bash
# Assembles the lanfirst menu-bar .app bundle from a prebuilt binary. Shared by
# install.sh (source builds) and the release workflow (CI builds) so the bundle
# layout is defined in exactly one place.
# Usage: make-app-bundle.sh <repo_dir> <app_dir> <lanfirst_binary> [version]
set -euo pipefail

REPO_DIR="$1"
APP_DIR="$2"
BIN="$3"
VERSION="${4:-0.0.0}"
VERSION="${VERSION#v}"   # CFBundleShortVersionString has no "v" prefix

mkdir -p "$APP_DIR/Contents/MacOS" "$APP_DIR/Contents/Resources"
install -m 755 "$BIN" "$APP_DIR/Contents/MacOS/lanfirst"

# Template menu-bar icons (committed PNGs from assets/render-icons.swift, with @2x
# Retina reps). menuet loads them by name via [NSImage imageNamed:] from the
# bundle's Resources dir and renders them as light/dark-aware template images.
cp "$REPO_DIR"/assets/icons/*.png "$APP_DIR/Contents/Resources/"
# App icon (assets/render-appicon.swift). The app is LSUIElement so it never
# shows in the Dock, but the icon appears in Finder, Get Info, and sharing UI.
cp "$REPO_DIR/assets/AppIcon.icns" "$APP_DIR/Contents/Resources/"

cat > "$APP_DIR/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>lanfirst</string>
  <key>CFBundleIdentifier</key><string>com.lanfirst.menubar</string>
  <key>CFBundleExecutable</key><string>lanfirst</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>LSUIElement</key><true/>
</dict></plist>
PLIST
