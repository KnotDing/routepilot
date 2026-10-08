#!/bin/sh
# Build Linux test executables on the Mac; execution can then be fully offline.
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"
export GOOS=linux GOARCH=arm64 CGO_ENABLED=0
mkdir -p bin/docker-tests
packages=$(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...)
for package in $packages; do
  name=$(printf '%s' "$package" | tr / _)
  go test -c -trimpath -o "bin/docker-tests/$name.test" "$package"
done
