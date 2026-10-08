#!/bin/sh
set -eu
if [ "$(id -u)" != 0 ]; then echo 'Administrator authorization required.' >&2; exit 1; fi
state_dir=${1:?temporary phase2 directory required}
case "$state_dir" in /private/tmp/surge-phase2.*) ;; *) echo 'Unexpected state directory' >&2; exit 1;; esac
for name in controller tcpdump egress-capture; do
  if [ -f "$state_dir/$name.pid" ]; then
    process_id=$(cat "$state_dir/$name.pid")
    command_line=$(ps -p "$process_id" -o command= || true)
    case "$name:$command_line" in
      controller:*surge-controller*verification-journal*"$state_dir"*) kill -TERM "$process_id" 2>/dev/null || true;;
      tcpdump:*tcpdump*"$state_dir/dns.pcap"*) kill -TERM "$process_id" 2>/dev/null || true;;
      egress-capture:*tcpdump*"$state_dir/egress.pcap"*) kill -TERM "$process_id" 2>/dev/null || true;;
    esac
  fi
done
/sbin/pfctl -a com.apple/surge-controller -F rules
/sbin/pfctl -a com.apple/surge-controller -F nat
if [ -s "$state_dir/pf-token" ]; then
  /sbin/pfctl -X "$(cat "$state_dir/pf-token")"
  : > "$state_dir/pf-token"
fi
echo 'Temporary test processes stopped; own PF reference released. Router test route needs manual cleanup.'
