#!/bin/sh
# Remove only generated release artifacts and abandoned packaging directories.
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
artifacts_dir="$project_dir/artifacts"
[ ! -L "$artifacts_dir" ] || { echo 'Artifacts directory must not be a symlink' >&2; exit 1; }
[ -d "$artifacts_dir" ] || exit 0
for output in "$artifacts_dir"/RoutePilot.app \
  "$artifacts_dir"/RoutePilot-macos-*.dmg "$artifacts_dir"/RoutePilot-macos-*.zip \
  "$artifacts_dir"/surge-controller-macos-*.tar.gz "$artifacts_dir"/SHA256SUMS \
  "$artifacts_dir"/package-verification.json \
  "$artifacts_dir"/app-build.* "$artifacts_dir"/app-previous.* \
  "$artifacts_dir"/surge-controller-macos.* "$artifacts_dir"/.DS_Store; do
  [ -e "$output" ] || [ -L "$output" ] || continue
  rm -rf -- "$output"
done
