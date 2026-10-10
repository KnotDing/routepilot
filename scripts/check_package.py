#!/usr/bin/env python3
"""Read-only validation of release artifacts; never installs or starts a service."""
import hashlib
import json
import pathlib
import platform
import plistlib
import re
import subprocess
import zipfile


RELEASE_VERSION = re.compile(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-beta\.[1-9][0-9]*)?")


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def valid_release_version(value):
    return RELEASE_VERSION.fullmatch(value) is not None


def checksum_entries(text):
    entries = {}
    for line in text.splitlines():
        match = re.fullmatch(r"([0-9a-fA-F]{64}) [ *](.+)", line)
        require(match is not None, "Invalid checksum line")
        digest, name = match.groups()
        require(name not in entries, "Duplicate checksum entry: " + name)
        entries[name] = digest.lower()
    require(entries, "Empty checksums")
    return entries


def verify_checksums(entries, files):
    require(set(entries) == set(files), "Checksum manifest does not cover exactly the expected files")
    for name, data in files.items():
        require(entries[name] == sha256(data), "Checksum mismatch: " + name)


def verify_identity(info, identity):
    require(info.get("CFBundleIdentifier") == "com.routepilot.app", "Wrong App identifier")
    require(info.get("CFBundleShortVersionString") == identity["version"], "Wrong App version")
    require(info.get("CFBundleVersion") == identity["build"], "Wrong App build")


def verify_notes(read):
    html = read("notes.html").decode("utf-8")
    require(not re.search(r'type\s*=\s*[\"\']module', html), "Offline notes must use a classic script")
    scripts = re.findall(r'<script[^>]+src=[\"\']([^\"\']+)', html)
    require(len(scripts) == 1, "Offline notes must have exactly one bundled script")
    for reference in scripts + re.findall(r'<link[^>]+href=[\"\']([^\"\']+)', html):
        require(reference.startswith("./assets/") and ".." not in reference[2:].split("/"), "Non-local notes asset")
        require(len(read(reference[2:])) > 0, "Empty notes asset")


def verify_service_resources(read):
    for name in ("bin/surge-controller", "scripts/install_macos.sh", "scripts/uninstall_macos.sh", "launchd/com.routepilot.surge-controller.plist"):
        require(len(read(name)) > 0, "Missing or empty bundled service resource: " + name)


def main():
    root = pathlib.Path(__file__).resolve().parent.parent
    artifacts = root / "artifacts"
    report = artifacts / "package-verification.json"
    report.unlink(missing_ok=True)  # A failed run must not leave an old success report.
    identity = json.loads((root / "internal/buildinfo/release.json").read_text())
    require(valid_release_version(identity["version"]), "Release must have a stable or numbered beta semantic version")
    arch = platform.machine()
    names = [f"RoutePilot-macos-{arch}.dmg", f"RoutePilot-macos-{arch}.zip"]
    files = {name: (artifacts / name).read_bytes() for name in names}
    require(all(files.values()), "Empty release artifact")
    checksums = checksum_entries((artifacts / "SHA256SUMS").read_text())
    verify_checksums(checksums, files)
    app = artifacts / "RoutePilot.app"
    verify_identity(plistlib.loads((app / "Contents/Info.plist").read_bytes()), identity)
    verify_notes(lambda name: (app / "Contents/Resources/ReleaseNotes" / name).read_bytes())
    service = app / "Contents/Resources/Service"
    verify_service_resources(lambda name: (service / name).read_bytes())
    backend = service / "bin/surge-controller"
    require(backend.stat().st_mode & 0o111, "Bundled service is not executable")
    with zipfile.ZipFile(artifacts / names[1]) as archive:
        require(len(archive.namelist()) == len(set(archive.namelist())), "Duplicate ZIP member")
        require(archive.testzip() is None, "Corrupt ZIP member")
        for file in app.rglob("*"):
            if file.is_file():
                require(archive.read(file.relative_to(artifacts).as_posix()) == file.read_bytes(), "ZIP differs from App: " + file.name)
        verify_identity(plistlib.loads(archive.read("RoutePilot.app/Contents/Info.plist")), identity)
        verify_notes(lambda name: archive.read("RoutePilot.app/Contents/Resources/ReleaseNotes/" + name))
    require(not list(artifacts.glob("surge-controller-macos-*.tar.gz")), "Obsolete backend archive remains in release output")
    version = json.loads(subprocess.check_output([str(backend), "version"], text=True))
    for key in ("name", "version", "build"):
        require(version[key] == identity[key], "Backend identity differs: " + key)
    require(version["os"] == "darwin" and version["arch"] == {"arm64": "arm64", "x86_64": "amd64"}[arch], "Wrong backend platform")
    for file in (app, backend):
        subprocess.run(["codesign", "--verify", "--deep", "--strict", str(file)], check=True)
    subprocess.run(["hdiutil", "verify", str(artifacts / names[0])], check=True)
    report.write_text(json.dumps({"passed": True, "identity": identity, "sha256": checksums, "checks": ["bundle identity", "backend identity", "offline notes", "ZIP/App equality", "bundled service resources", "outer checksums", "code signatures", "DMG integrity"], "scope": "Read-only package validation; no installation, notarization or live upgrade acceptance"}, indent=2) + "\n")
    print("PASS package: versions, resources, archives, checksums, signatures, DMG")


if __name__ == "__main__":
    main()
