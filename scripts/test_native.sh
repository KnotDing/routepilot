#!/bin/sh
# Isolated AppKit/WebKit checks: no backend, PF, Keychain or updater is started.
set -eu
cd "$(dirname "$0")/.."
[ "$(uname -s)" = Darwin ] || { echo 'Native checks require macOS' >&2; exit 1; }
mkdir -p .cache/native-tests .cache/swift-module
compile() {
  name=$1; shift
  xcrun swiftc -parse-as-library -swift-version 5 -module-cache-path .cache/swift-module "$@" -o ".cache/native-tests/$name"
}
compile installer native/installer.swift native/installer_test.swift
.cache/native-tests/installer
compile localization native/localization.swift native/localization_test.swift
.cache/native-tests/localization localization
compile core -framework AppKit -framework WebKit -framework Security native/test_support.swift native/localization.swift native/release_updater.swift native/update_assistant.swift native/status_icon.swift native/status_preferences.swift native/credentials.swift native/automation.swift native/core_test.swift
.cache/native-tests/core
compile ui -framework AppKit -framework WebKit native/test_support.swift native/localization.swift native/release_updater.swift native/window_frame.swift native/update_dialog.swift native/ui_test.swift
.cache/native-tests/ui "$(pwd)/web/dist-native" > .cache/native-tests/ui.log 2>&1
cat .cache/native-tests/ui.log
grep -q "^PASS native UI:" .cache/native-tests/ui.log
