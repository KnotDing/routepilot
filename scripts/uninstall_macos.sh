#!/bin/sh
# Stop the service; preserve SQLite, encryption key and private logs.
set -eu
[ "$(uname -s)" = Darwin ] || { echo 'macOS is required' >&2; exit 1; }
[ "$(id -u)" = 0 ] || { echo 'Run this uninstaller with sudo' >&2; exit 1; }
update_pid=$(/bin/launchctl print system/com.routepilot.update-worker 2>/dev/null | /usr/bin/awk '/^[[:space:]]*pid =/ { print $3; exit }' || true)
[ -z "$update_pid" ] || { echo 'An online backend update is in progress; wait for it to finish' >&2; exit 1; }
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
/bin/launchctl bootout system/com.routepilot.update-worker 2>/dev/null || true
[ ! -e /Library/LaunchDaemons/com.routepilot.update-worker.plist ] || /bin/rm /Library/LaunchDaemons/com.routepilot.update-worker.plist
[ ! -d /usr/local/libexec/surge-controller/.release-update ] || /bin/rm -rf /usr/local/libexec/surge-controller/.release-update
[ ! -e /var/db/surge-controller/software-update-sessions.json ] || /bin/rm /var/db/surge-controller/software-update-sessions.json
echo 'Uninstalled. Configuration and logs are retained; only the Controller PF lease is released.'
