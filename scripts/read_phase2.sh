#!/bin/sh
set -eu
state_dir=${1:?phase2 directory required}
case "$state_dir" in /private/tmp/surge-phase2.*) ;; *) exit 1;; esac
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
reader_uid=$(stat -f '%u' "$project_dir/bin/surge-controller")
# Expose only the explicitly requested verification data to its local owner.
# PID and PF-reference files remain root-owned and private.
chmod 0711 "$state_dir"
for name in journal.jsonl controller.log tcpdump.log dns.pcap egress.pcap egress-capture.log; do
  if [ -f "$state_dir/$name" ]; then chown "$reader_uid" "$state_dir/$name"; chmod 0600 "$state_dir/$name"; fi
done
/sbin/pfctl -s Anchors
/sbin/pfctl -sr
/sbin/pfctl -sn
/sbin/pfctl -a com.apple/surge-controller -vvsr
/sbin/pfctl -a com.apple/surge-controller -vvsn
