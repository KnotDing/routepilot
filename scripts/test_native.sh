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
compile monitoring native/monitoring_preferences.swift native/monitoring_preferences_test.swift
.cache/native-tests/monitoring
compile localization native/localization.swift native/localization_test.swift
.cache/native-tests/localization localization
compile backend-menu -framework AppKit native/test_support.swift native/localization.swift native/release_updater.swift native/management_backends.swift native/management_backends_test.swift
.cache/native-tests/backend-menu
compile core -framework AppKit -framework WebKit -framework Security native/test_support.swift native/localization.swift native/release_updater.swift native/update_assistant.swift native/status_icon.swift native/status_preferences.swift native/credentials.swift native/automation.swift native/core_test.swift
.cache/native-tests/core
compile remote-restart -framework AppKit -framework WebKit -framework UserNotifications native/test_support.swift native/localization.swift native/release_updater.swift native/window_frame.swift native/status_preferences.swift native/monitoring_preferences.swift native/automation.swift native/notifications.swift native/remote_update_restart.swift native/remote_update_restart_test.swift
.cache/native-tests/remote-restart
compile ui -framework AppKit -framework WebKit native/test_support.swift native/localization.swift native/release_updater.swift native/window_frame.swift native/update_dialog.swift native/ui_test.swift
.cache/native-tests/ui "$(pwd)/web/dist-native" > .cache/native-tests/ui.log 2>&1
cat .cache/native-tests/ui.log
grep -q "^PASS native UI:" .cache/native-tests/ui.log

# Exercise the production AppDelegate with isolated loopback HTML/API fixtures.
# The test build excludes the App entry point and real preference/vault seeds.
set -eu
python3 - <<'PYCOMPILE'
import pathlib,re,subprocess
line=next(row for row in pathlib.Path('scripts/build_app.sh').read_text().splitlines() if row.startswith('xcrun swiftc -parse-as-library'))
sources=re.findall(r'native/[a-z_]+\.swift',line)
command=['xcrun','swiftc','-parse-as-library','-swift-version','5','-D','ROUTEPILOT_NATIVE_TEST','-module-cache-path','.cache/swift-module']
for framework in ['AppKit','WebKit','ServiceManagement','UserNotifications']:command+=['-framework',framework]
subprocess.run(command+sources+['native/management_windows_test.swift','-o','.cache/native-tests/management-windows'],check=True)
PYCOMPILE
fixture_port=$(mktemp .cache/native-tests/management-port.XXXXXX)
python3 native/management_windows_fixture.py "$fixture_port" &
fixture_pid=$!
trap 'kill "$fixture_pid" 2>/dev/null || true; rm -f "$fixture_port"' EXIT HUP INT TERM
while [ ! -s "$fixture_port" ]; do kill -0 "$fixture_pid"; sleep 0.1; done
.cache/native-tests/management-windows "$(cat "$fixture_port")" --smoke-test
