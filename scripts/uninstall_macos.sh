#!/bin/sh
# Stop the service; preserve SQLite, encryption key and private logs.
set -eu
[ "$(uname -s)" = Darwin ] || { echo 'macOS is required' >&2; exit 1; }
[ "$(id -u)" = 0 ] || { echo 'Run this uninstaller with sudo' >&2; exit 1; }
service_plist=/Library/LaunchDaemons/com.routepilot.surge-controller.plist
service_bin=/usr/local/libexec/surge-controller/surge-controller
if /bin/launchctl print system/com.routepilot.surge-controller >/dev/null 2>&1; then
  /bin/launchctl bootout system/com.routepilot.surge-controller
fi
if [ -e /var/db/surge-controller/pf-lease.json ]; then
  "$service_bin" cleanup-pf --state-directory /var/db/surge-controller
fi
[ ! -e "$service_plist" ] || /bin/rm "$service_plist"
[ ! -e "$service_bin" ] || /bin/rm "$service_bin"
echo 'Uninstalled. Configuration and logs are retained; only the Controller PF lease is released.'
