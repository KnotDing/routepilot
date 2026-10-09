import unittest
import gzip
import io
import pathlib
import tarfile
import tempfile

from check_package import checksum_entries, sha256, verify_backend_archive, verify_checksums, verify_identity, verify_notes


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

    def test_backend_archive(self):
        binaries = {"surge-controller": b"controller", "dns-probe": b"probe"}
        files = {"bin/" + name: data for name, data in binaries.items()}
        files["SHA256SUMS"] = "".join(sha256(data) + "  " + name + "\n" for name, data in files.items()).encode()
        for name in ("scripts/install_macos.sh", "scripts/update_macos.sh", "scripts/uninstall_macos.sh", "launchd/com.routepilot.surge-controller.plist"):
            files[name] = b"resource"
        def pack(extra=None):
            output = io.BytesIO()
            with tarfile.open(fileobj=output, mode="w:gz") as archive:
                for name, data in [*files.items(), *(extra or [])]:
                    member = tarfile.TarInfo("./" + name)
                    member.size = len(data)
                    member.mode = 0o755
                    archive.addfile(member, io.BytesIO(data))
            return output.getvalue()
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "backend.tar.gz"
            path.write_bytes(pack())
            verify_backend_archive(path, binaries)
            for extra in [[("bin/._surge-controller", b"metadata")], [("../escape", b"bad")], [("bin/dns-probe", b"duplicate")]]:
                with self.subTest(extra=extra), self.assertRaises(ValueError):
                    path.write_bytes(pack(extra))
                    verify_backend_archive(path, binaries)
            # tar EOF must not hide a broken gzip footer.
            contents = bytearray(pack()); contents[-8] ^= 1
            path.write_bytes(contents)
            with self.assertRaises(gzip.BadGzipFile):
                verify_backend_archive(path, binaries)


if __name__ == "__main__":
    unittest.main()
