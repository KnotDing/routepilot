#!/bin/sh
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"
[ "$(uname -s)" = Darwin ] || { echo 'Build the APP on macOS' >&2; exit 1; }
sh "$project_dir/scripts/clean_artifacts.sh"
mkdir -p "$project_dir/artifacts" "$project_dir/.cache"
build_dir=$(mktemp -d "$project_dir/.cache/app-build.XXXXXX")
trap 'rm -rf "$build_dir"' EXIT
app_dir="$build_dir/RoutePilot.app"
mkdir -p "$app_dir/Contents/MacOS" "$app_dir/Contents/Resources/Service/bin" "$app_dir/Contents/Resources/Service/scripts" "$app_dir/Contents/Resources/Service/launchd" .cache/swift-module .cache/AppIcon.iconset
xcrun swiftc -parse-as-library -swift-version 5 -target "$(uname -m)-apple-macos13.0" -module-cache-path "$project_dir/.cache/swift-module" -framework AppKit -framework WebKit -framework ServiceManagement -framework UserNotifications native/main.swift native/window_frame.swift native/status_icon.swift native/credentials.swift native/remote_credentials.swift native/notifications.swift native/installer.swift native/localization.swift native/release_updater.swift native/update_assistant.swift native/update_dialog.swift -o "$app_dir/Contents/MacOS/RoutePilot"
cp native/Info.plist "$app_dir/Contents/Info.plist"
# Keep the native bundle aligned with the release identity embedded in Go and Vite.
release_version=$(plutil -extract version raw -o - internal/buildinfo/release.json)
release_build=$(plutil -extract build raw -o - internal/buildinfo/release.json)
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $release_version" "$app_dir/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $release_build" "$app_dir/Contents/Info.plist"
xcrun swiftc -module-cache-path "$project_dir/.cache/swift-module" native/icon.swift -o .cache/create-icon
.cache/create-icon "$project_dir/.cache/AppIcon.iconset" "$app_dir/Contents/Resources/MenuIcon.png"
if ! iconutil -c icns .cache/AppIcon.iconset -o "$app_dir/Contents/Resources/AppIcon.icns"; then
  python3 scripts/pack_icon.py .cache/AppIcon.iconset "$app_dir/Contents/Resources/AppIcon.icns"
fi
cp bin/surge-controller "$app_dir/Contents/Resources/Service/bin/"
cp scripts/install_macos.sh scripts/uninstall_macos.sh scripts/update_macos.sh "$app_dir/Contents/Resources/Service/scripts/"
cp launchd/com.routepilot.surge-controller.plist "$app_dir/Contents/Resources/Service/launchd/"
cp -R web/dist-native "$app_dir/Contents/Resources/ReleaseNotes"
cp README.md "$app_dir/Contents/Resources/README.md"
cp localization/*.json "$app_dir/Contents/Resources/"
cp internal/config/notification-events.json "$app_dir/Contents/Resources/"
cp -R native/en.lproj native/zh-Hans.lproj "$app_dir/Contents/Resources/"
codesign --force --sign - "$app_dir/Contents/Resources/Service/bin/surge-controller"
codesign --force --sign - "$app_dir"
codesign --verify --deep --strict "$app_dir"
if [ -d "$project_dir/artifacts/RoutePilot.app" ]; then
  old_dir=$(mktemp -d "$project_dir/.cache/app-previous.XXXXXX")
  mv "$project_dir/artifacts/RoutePilot.app" "$old_dir/"
fi
mv "$app_dir" "$project_dir/artifacts/RoutePilot.app"
app_dir="$project_dir/artifacts/RoutePilot.app"
if [ -n "${old_dir:-}" ]; then rm -rf "$old_dir"; fi
ditto -c -k --sequesterRsrc --keepParent "$app_dir" "artifacts/RoutePilot-macos-$(uname -m).zip"
dmg_stage=$(mktemp -d "$project_dir/.cache/dmg-stage.XXXXXX")
trap 'rm -rf "$build_dir" "$dmg_stage"' EXIT
ditto "$app_dir" "$dmg_stage/RoutePilot.app"
ln -s /Applications "$dmg_stage/Applications"
hdiutil create -volname RoutePilot -srcfolder "$dmg_stage" -ov -format UDZO "artifacts/RoutePilot-macos-$(uname -m).dmg"
printf 'APP: %s\n' "$app_dir"
