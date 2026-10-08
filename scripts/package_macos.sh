#!/bin/sh
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"
[ "$(uname -s)" = Darwin ] || { echo 'Build the release on macOS' >&2; exit 1; }
mkdir -p artifacts
package_dir=$(mktemp -d "$project_dir/artifacts/surge-controller-macos.XXXXXX")
mkdir -p "$package_dir/bin" "$package_dir/scripts" "$package_dir/launchd" "$package_dir/docs"
cp bin/surge-controller bin/dns-probe "$package_dir/bin/"
cp scripts/install_macos.sh scripts/uninstall_macos.sh scripts/update_macos.sh "$package_dir/scripts/"
cp launchd/com.routepilot.surge-controller.plist "$package_dir/launchd/"
cp README.md "$package_dir/"
cp docs/*.md "$package_dir/docs/"
(cd "$package_dir" && shasum -a 256 bin/* > SHA256SUMS)
archive="$project_dir/artifacts/surge-controller-macos-$(uname -m).tar.gz"
tar -czf "$archive" -C "$package_dir" .
printf 'Release: %s\n' "$archive"
