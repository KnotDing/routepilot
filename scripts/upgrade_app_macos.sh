#!/bin/sh
# Replace the RoutePilot app bundle and, when installed, its launchd backend.
set -eu
[ "$(uname -s)" = Darwin ] || { echo 'macOS is required' >&2; exit 1; }
[ "$(id -u)" = 0 ] || { echo 'Administrator authorization is required' >&2; exit 1; }
[ "$#" = 2 ] || { echo 'Expected a staged app and installed app path' >&2; exit 1; }
source_app=$1
target_app=$2
[ "$(basename "$source_app")" = RoutePilot.app ] || { echo 'Invalid update app bundle' >&2; exit 1; }
[ "$(basename "$target_app")" = RoutePilot.app ] || { echo 'Invalid installed app path' >&2; exit 1; }
[ -d "$source_app" ] && [ ! -L "$source_app" ] || { echo 'Update app bundle is missing or unsafe' >&2; exit 1; }
[ -d "$target_app" ] && [ ! -L "$target_app" ] || { echo 'Installed app bundle is missing or unsafe' >&2; exit 1; }
[ -f "$source_app/Contents/Info.plist" ] && [ ! -L "$source_app/Contents/Info.plist" ] || { echo 'Update app metadata is invalid' >&2; exit 1; }
[ -f "$source_app/Contents/Resources/Service/bin/surge-controller" ] && [ ! -L "$source_app/Contents/Resources/Service/bin/surge-controller" ] || { echo 'Update service executable is missing' >&2; exit 1; }
/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$source_app/Contents/Info.plist" | /usr/bin/grep -qx 'com.routepilot.app'
/usr/bin/codesign --verify --deep --strict "$source_app"
parent=$(dirname "$target_app")
[ -d "$parent" ] && [ ! -L "$parent" ] || { echo 'Installed app parent directory is unsafe' >&2; exit 1; }
stage=$(/usr/bin/mktemp -d "$parent/.RoutePilot-upgrade.XXXXXX")
new_app="$stage/RoutePilot.app"
backup="$parent/.RoutePilot-backup.$$"
swapped=0
cleanup() {
  result=$?
  if [ "$swapped" = 1 ] && [ ! -e "$target_app" ] && [ -e "$backup" ]; then /bin/mv "$backup" "$target_app"; fi
  /bin/rm -rf "$stage"
  exit "$result"
}
trap cleanup EXIT
/usr/bin/ditto "$source_app" "$new_app"
/usr/bin/codesign --verify --deep --strict "$new_app"
if [ -f /Library/LaunchDaemons/com.routepilot.surge-controller.plist ]; then
  /bin/sh "$source_app/Contents/Resources/Service/scripts/update_macos.sh"
fi
/bin/mv "$target_app" "$backup"
swapped=1
if /bin/mv "$new_app" "$target_app"; then
  /bin/rm -rf "$backup"
  swapped=0
else
  /bin/mv "$backup" "$target_app"
  swapped=0
  echo 'Could not replace the installed app bundle' >&2
  exit 1
fi
/bin/rm -rf "$stage"
trap - EXIT
echo 'RoutePilot app updated; configuration and database preserved.'
