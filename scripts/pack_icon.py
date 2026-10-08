#!/usr/bin/env python3
"""Package standard PNG representations as ICNS when iconutil is unavailable."""
import struct
import sys
from pathlib import Path


def pack(iconset: Path, destination: Path) -> None:
    representations = [
        ("icp4", "icon_16x16.png", 16), ("ic11", "icon_16x16@2x.png", 32),
        ("icp5", "icon_32x32.png", 32), ("ic12", "icon_32x32@2x.png", 64),
        ("ic07", "icon_128x128.png", 128), ("ic13", "icon_128x128@2x.png", 256),
        ("ic08", "icon_256x256.png", 256), ("ic14", "icon_256x256@2x.png", 512),
        ("ic09", "icon_512x512.png", 512), ("ic10", "icon_512x512@2x.png", 1024),
    ]
    chunks = []
    for kind, filename, size in representations:
        data = (iconset / filename).read_bytes()
        if data[:8] != b"\x89PNG\r\n\x1a\n" or data[12:16] != b"IHDR" or struct.unpack(">II", data[16:24]) != (size, size):
            raise ValueError(f"Invalid {size}px icon representation: {filename}")
        chunks.append(kind.encode("ascii") + struct.pack(">I", len(data) + 8) + data)
    payload = b"".join(chunks)
    destination.write_bytes(b"icns" + struct.pack(">I", len(payload) + 8) + payload)


if __name__ == "__main__":
    pack(Path(sys.argv[1]), Path(sys.argv[2]))
