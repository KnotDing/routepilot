#!/bin/sh
# Temporary root-run Phase 2 harness. Never modifies router, Surge or root PF.
set -eu
umask 077
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
anchor=com.apple/surge-controller
test_profile=${1:-lan}
case "$test_profile" in
  lan) capture_interface=en0; rule_file=macos-test.pf;;
  docker) capture_interface=bridge101; rule_file=macos-docker-test.pf;;
  *) echo 'Expected lan or docker profile' >&2; exit 1;;
esac
if [ "$(id -u)" != 0 ]; then
  echo 'Run through native macOS administrator authorization.' >&2
  exit 1
fi
existing=$(/sbin/pfctl -a "$anchor" -sr 2>/dev/null)
existing_nat=$(/sbin/pfctl -a "$anchor" -sn 2>/dev/null)
if [ -n "$existing" ] || [ -n "$existing_nat" ]; then
  echo 'This anchor already has rules; refusing to replace an active instance.' >&2
  exit 1
fi
root_rules=$(/sbin/pfctl -sr 2>/dev/null)
root_nat=$(/sbin/pfctl -sn 2>/dev/null)
case "$root_rules" in *'anchor "com.apple/*"'*) ;; *) echo 'Root filter anchor does not reach com.apple/*' >&2; exit 1;; esac
case "$root_nat" in *'rdr-anchor "com.apple/*"'*) ;; *) echo 'Root rdr anchor does not reach com.apple/*' >&2; exit 1;; esac
/sbin/pfctl -n -a "$anchor" -f "$project_dir/docs/$rule_file"
state_dir=$(mktemp -d /private/tmp/surge-phase2.XXXXXX)
reference=$(/sbin/pfctl -E 2>&1)
printf '%s\n' "$reference" | sed -n 's/.*Token : //p' > "$state_dir/pf-token"
if [ ! -s "$state_dir/pf-token" ]; then
  printf '%s\n' "$reference" >&2
  echo 'No PF enable reference token captured; aborting.' >&2
  exit 1
fi
cleanup_failure() {
  for process_name in controller tcpdump egress-capture; do
    if [ -f "$state_dir/$process_name.pid" ]; then
      kill -TERM "$(cat "$state_dir/$process_name.pid")" 2>/dev/null || true
    fi
  done
  /sbin/pfctl -a "$anchor" -F rules >/dev/null 2>&1 || true
  /sbin/pfctl -a "$anchor" -F nat >/dev/null 2>&1 || true
  /sbin/pfctl -X "$(cat "$state_dir/pf-token")" >/dev/null 2>&1 || true
}
trap cleanup_failure EXIT
/sbin/pfctl -a "$anchor" -f "$project_dir/docs/$rule_file"
trap '' HUP
"$project_dir/bin/surge-controller" \
  --listen=192.168.20.2:1053 --transparent-listen=192.168.20.2:1054 \
  --egress-interface=en0 --egress-source4=192.168.20.2 \
  --intercept-prefix=8.8.8.8/32 --gateway-bypass-verified --mode=force-direct \
  --verification-journal="$state_dir/journal.jsonl" \
  > "$state_dir/controller.log" 2>&1 < /dev/null &
echo "$!" > "$state_dir/controller.pid"
/usr/sbin/tcpdump -U -n -i "$capture_interface" -w "$state_dir/dns.pcap" \
  'host 8.8.8.8 and port 53' > "$state_dir/tcpdump.log" 2>&1 < /dev/null &
echo "$!" > "$state_dir/tcpdump.pid"
if [ "$test_profile" = docker ]; then
  /usr/sbin/tcpdump -U -n -i en0 -w "$state_dir/egress.pcap" \
    'host 8.8.8.8 and port 53' > "$state_dir/egress-capture.log" 2>&1 < /dev/null &
  echo "$!" > "$state_dir/egress-capture.pid"
fi
sleep 1
if ! kill -0 "$(cat "$state_dir/controller.pid")" 2>/dev/null; then
  cat "$state_dir/controller.log" >&2
  kill -TERM "$(cat "$state_dir/tcpdump.pid")" 2>/dev/null || true
  exit 1
fi
trap - EXIT
printf '%s\n' "$state_dir"
