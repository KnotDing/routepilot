#!/bin/sh
set -eu
for test_binary in /tests/*.test; do
  printf '\nRunning %s\n' "$test_binary"
  "$test_binary" -test.v -test.timeout=30s
done
