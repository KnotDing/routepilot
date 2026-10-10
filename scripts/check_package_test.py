import unittest

from check_package import checksum_entries, sha256, verify_service_resources, verify_checksums, verify_identity, verify_notes


class PackageChecks(unittest.TestCase):
    def test_checksums(self):
        data = b"package"
        manifest = sha256(data).upper() + " *release.zip\n"
        verify_checksums(checksum_entries(manifest), {"release.zip": data})
        for invalid in ("", "bad", manifest + manifest, "g" * 64 + "  release.zip"):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                checksum_entries(invalid)
        for files in ({"release.zip": b"tampered"}, {}, {"other": data}):
            with self.subTest(files=files), self.assertRaises(ValueError):
                verify_checksums(checksum_entries(manifest), files)

    def test_identity(self):
        release = {"version": "1.0.0", "build": "1"}
        info = {"CFBundleIdentifier": "com.routepilot.app", "CFBundleShortVersionString": "1.0.0", "CFBundleVersion": "1"}
        verify_identity(info, release)
        for key in info:
            with self.subTest(key=key), self.assertRaises(ValueError):
                verify_identity({**info, key: "wrong"}, release)

    def test_offline_notes(self):
        def read(html):
            return lambda name: html.encode() if name == "notes.html" else b"bundle"
        verify_notes(read('<script defer src="./assets/notes.js"></script>'))
        for html in ('<script type="module" src="./assets/notes.js"></script>', '<script src="https://example.com/notes.js"></script>', '<script src="./assets/../secret.js"></script>', '<html></html>'):
            with self.subTest(html=html), self.assertRaises(ValueError):
                verify_notes(read(html))

    def test_bundled_service_resources(self):
        files = {name: b"resource" for name in ("bin/surge-controller", "scripts/install_macos.sh", "scripts/uninstall_macos.sh", "launchd/com.routepilot.surge-controller.plist")}
        verify_service_resources(files.__getitem__)
        for name in files:
            with self.subTest(name=name), self.assertRaises(ValueError):
                verify_service_resources({**files, name: b""}.__getitem__)
            missing = {key:value for key,value in files.items() if key!=name}
            with self.subTest(missing=name), self.assertRaises(KeyError):
                verify_service_resources(missing.__getitem__)



if __name__ == "__main__":
    unittest.main()
