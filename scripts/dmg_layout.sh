#!/bin/sh
# Regenerate the committed DMG Finder layout fixture (scripts/dmg/DS_Store).
# Run manually on a Mac with a logged-in Finder session after changing the DMG
# window layout; icon positions must match the artwork in native/dmg_background.swift.
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"
stage=$(mktemp -d)
mounted=""
cleanup() {
  [ -z "$mounted" ] || hdiutil detach "$mounted" 2>/dev/null || hdiutil detach -force "$mounted" 2>/dev/null || true
  rm -rf "$stage"
}
trap cleanup EXIT
mkdir -p "$stage/src/RoutePilot.app/Contents" "$stage/src/.background"
ln -s /Applications "$stage/src/Applications"
xcrun swiftc -swift-version 5 -module-cache-path "$project_dir/.cache/swift-module" -framework AppKit "$project_dir/native/dmg_background.swift" -o "$stage/create-background"
"$stage/create-background" "$stage/src/.background/background.pdf"
hdiutil create -volname RoutePilot -srcfolder "$stage/src" -ov -format UDRW "$stage/layout.dmg" >/dev/null 2>&1
# Finder only tracks volumes mounted under /Volumes; address the mounted name in
# case another RoutePilot volume forces a "RoutePilot N" suffix.
mounted=$(hdiutil attach "$stage/layout.dmg" | awk 'index($0, "/Volumes/") {print substr($0, index($0, "/Volumes/")); exit}')
[ -n "$mounted" ] || { echo 'Could not find mounted DMG volume' >&2; exit 1; }
name=$(basename "$mounted")
sleep 2
osascript <<APPLESCRIPT
with timeout of 300 seconds
tell application "Finder"
  tell disk "$name"
    open
    set current view of container window to icon view
    set toolbar visible of container window to false
    set statusbar visible of container window to false
    set the bounds of container window to {80, 80, 740, 500}
    set viewOptions to the icon view options of container window
    set arrangement of viewOptions to not arranged
    set icon size of viewOptions to 128
    set text size of viewOptions to 14
    set background picture of viewOptions to (POSIX file "$mounted/.background/background.pdf") as alias
    set position of item "RoutePilot.app" of container window to {180, 165}
    set position of item "Applications" of container window to {480, 165}
    update without registering applications
    delay 2
    close
  end tell
end tell
end timeout
APPLESCRIPT
hdiutil detach "$mounted" >/dev/null
mounted=""
hdiutil attach -readonly -nobrowse -mountpoint "$stage/mnt" "$stage/layout.dmg" >/dev/null 2>&1
mounted="$stage/mnt"
[ -s "$stage/mnt/.DS_Store" ] || { echo 'Finder did not write .DS_Store' >&2; exit 1; }
cp "$stage/mnt/.DS_Store" "$project_dir/scripts/dmg/DS_Store"
echo "Wrote scripts/dmg/DS_Store"
