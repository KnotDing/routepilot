#!/bin/sh
# Update only the executable; preserve the installed database, key and settings.
set -eu
[ "$(uname -s)" = Darwin ] || { echo 'macOS is required' >&2; exit 1; }
[ "$(id -u)" = 0 ] || { echo 'Administrator authorization is required' >&2; exit 1; }
umask 077
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
service_dir=/usr/local/libexec/surge-controller
service_bin=$service_dir/surge-controller
service_plist=/Library/LaunchDaemons/com.routepilot.surge-controller.plist
for regular in "$project_dir/bin/surge-controller" "$service_bin" "$service_plist"; do
  [ -f "$regular" ] && [ ! -L "$regular" ] || { echo 'A regular existing service and executable are required' >&2; exit 1; }
done
[ ! -L "$service_dir" ] || { echo 'Service directory must not be a symlink' >&2; exit 1; }
/usr/bin/codesign --verify --strict "$project_dir/bin/surge-controller"
staging=$(/usr/bin/mktemp -d "$service_dir/.update.XXXXXX")
stopped=0
committed=0
cleanup() {
  result=$?
  if [ "$stopped" = 1 ] && [ "$committed" = 0 ]; then
    /usr/bin/install -o root -g wheel -m 0755 "$staging/previous" "$service_bin"
    /bin/launchctl bootstrap system "$service_plist" || true
  fi
  /bin/rm -rf "$staging"
  exit "$result"
}
trap cleanup EXIT
/usr/bin/install -o root -g wheel -m 0755 "$project_dir/bin/surge-controller" "$staging/next"
/usr/bin/install -o root -g wheel -m 0755 "$service_bin" "$staging/previous"
previous_pid=$(/bin/launchctl print system/com.routepilot.surge-controller | /usr/bin/awk '/^[[:space:]]*pid =/ { print $3; exit }')
/bin/launchctl bootout system "$service_plist"
stopped=1
if [ -n "$previous_pid" ]; then
  attempts=0
  while /bin/kill -0 "$previous_pid" 2>/dev/null; do
    attempts=$((attempts + 1))
    [ "$attempts" -lt 30 ] || { echo 'Previous service has not stopped; update cancelled' >&2; exit 1; }
    /bin/sleep 1
  done
fi
/bin/mv "$staging/next" "$service_bin"
/bin/launchctl bootstrap system "$service_plist"
committed=1
echo 'Network service updated; database, key and configuration preserved.'
