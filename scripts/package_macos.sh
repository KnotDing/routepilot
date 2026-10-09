#!/bin/sh
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"
[ "$(uname -s)" = Darwin ] || { echo 'Build the release on macOS' >&2; exit 1; }
mkdir -p artifacts .cache
[ ! -L "$project_dir/artifacts" ] || { echo 'Artifacts directory must not be a symlink' >&2; exit 1; }
# Preserve the App/DMG/ZIP produced by the preceding app build.
for previous in "$project_dir/artifacts"/surge-controller-macos-*.tar.gz "$project_dir/artifacts"/surge-controller-macos.* "$project_dir/artifacts"/SHA256SUMS; do
  [ -e "$previous" ] || [ -L "$previous" ] || continue
  rm -rf -- "$previous"
done
package_dir=$(mktemp -d "$project_dir/.cache/surge-controller-macos.XXXXXX")
trap 'rm -rf "$package_dir"' EXIT
mkdir -p "$package_dir/bin" "$package_dir/scripts" "$package_dir/launchd" "$package_dir/docs"
codesign --force --sign - bin/surge-controller bin/dns-probe
cp bin/surge-controller bin/dns-probe "$package_dir/bin/"
cp scripts/install_macos.sh scripts/uninstall_macos.sh scripts/update_macos.sh "$package_dir/scripts/"
cp launchd/com.routepilot.surge-controller.plist "$package_dir/launchd/"
cp README.md "$package_dir/"
cp docs/*.md "$package_dir/docs/"
(cd "$package_dir" && shasum -a 256 bin/* > SHA256SUMS)
archive="$project_dir/artifacts/surge-controller-macos-$(uname -m).tar.gz"
tar -czf "$archive" -C "$package_dir" .
printf 'Release: %s\n' "$archive"
