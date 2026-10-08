#!/bin/sh
# Explicit operator action: install a new service; preserve existing private data.
set -eu
[ "$(uname -s)" = Darwin ] || { echo 'macOS is required' >&2; exit 1; }
[ "$(id -u)" = 0 ] || { echo 'Run this installer with sudo' >&2; exit 1; }
umask 077
source_dir=
if [ "$#" -gt 0 ]; then
  [ "$#" = 2 ] && [ "$1" = --data-directory ] || { echo 'Unknown installer arguments' >&2; exit 1; }
  source_dir=$2
  case "$source_dir" in /*) ;; *) echo 'Data directory must be absolute' >&2; exit 1 ;; esac
  [ ! -L "$source_dir" ] || { echo 'Source must not be a symlink' >&2; exit 1; }
  for source_file in controller.sqlite controller.sqlite.key; do
    [ -f "$source_dir/$source_file" ] && [ ! -L "$source_dir/$source_file" ] || { echo 'Database and key must be regular files' >&2; exit 1; }
  done
  [ ! -e /var/db/surge-controller/controller.sqlite ] && [ ! -e /var/db/surge-controller/controller.sqlite.key ] || { echo 'Existing service data retained; migration refused' >&2; exit 1; }
  [ ! -e "$source_dir/controller.sqlite-wal" ] || { echo 'Stop the Controller cleanly before migration' >&2; exit 1; }
fi
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
service_plist=/Library/LaunchDaemons/com.routepilot.surge-controller.plist
service_bin=/usr/local/libexec/surge-controller/surge-controller
[ -x "$project_dir/bin/surge-controller" ] || { echo 'Build or unpack the macOS executable first' >&2; exit 1; }
[ ! -e "$service_plist" ] && [ ! -e "$service_bin" ] || { echo 'Service already installed; uninstall it first (data is retained)' >&2; exit 1; }
for service_dir in /var/db/surge-controller /var/log/surge-controller /usr/local/libexec/surge-controller; do
  [ ! -L "$service_dir" ] || { echo 'Service directories must not be symlinks' >&2; exit 1; }
  /usr/bin/install -d -o root -g wheel -m 0700 "$service_dir"
done
if [ -n "$source_dir" ]; then
  /usr/bin/install -o root -g wheel -m 0600 "$source_dir/controller.sqlite" /var/db/surge-controller/controller.sqlite
  /usr/bin/install -o root -g wheel -m 0600 "$source_dir/controller.sqlite.key" /var/db/surge-controller/controller.sqlite.key
fi
/usr/bin/install -o root -g wheel -m 0755 "$project_dir/bin/surge-controller" "$service_bin"
/usr/bin/install -o root -g wheel -m 0644 "$project_dir/launchd/com.routepilot.surge-controller.plist" "$service_plist"
/usr/bin/plutil -lint "$service_plist"
/bin/launchctl bootstrap system "$service_plist"
echo 'Installed. Open http://127.0.0.1:1055 and configure DNS egress and BGP before enabling interception.'
