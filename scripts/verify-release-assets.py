#!/usr/bin/env python3
"""Validate the exact two-file public release; hashes stay in CI logs."""
from __future__ import annotations
import argparse
import hashlib
import json
import re
import struct
from pathlib import Path

WINDOWS_ASSETS = frozenset({"ProxyStart-windows-amd64.exe"})
MACOS_ASSETS = frozenset({"ProxyStart-macos-universal.dmg"})
RELEASE_ASSETS = WINDOWS_ASSETS | MACOS_ASSETS

def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()

def verify_directory(directory: Path, version: str) -> dict[str, str]:
    if re.fullmatch(r"\d+\.\d+\.\d+", version) is None:
        raise ValueError("version must be a three-part numeric version")
    directory = directory.resolve()
    if not directory.is_dir():
        raise ValueError("release directory does not exist")
    actual = {entry.name for entry in directory.iterdir()}
    if actual != RELEASE_ASSETS or any(not p.is_file() or p.is_symlink() for p in directory.iterdir()):
        raise ValueError(f"release asset set is {sorted(actual)}, expected {sorted(RELEASE_ASSETS)}")
    exe = (directory / "ProxyStart-windows-amd64.exe").read_bytes()
    if len(exe) < 64 or exe[:2] != b"MZ":
        raise ValueError("invalid Windows executable")
    offset = struct.unpack_from("<I", exe, 0x3c)[0]
    if offset + 94 > len(exe) or exe[offset:offset+4] != b"PE\0\0":
        raise ValueError("invalid Windows PE header")
    if struct.unpack_from("<H", exe, offset+4)[0] != 0x8664 or struct.unpack_from("<H", exe, offset+92)[0] != 2:
        raise ValueError("Windows release must be an x64 GUI executable")
    with (directory / "ProxyStart-macos-universal.dmg").open("rb") as stream:
        if stream.seek(0, 2) < 512:
            raise ValueError("invalid macOS DMG")
        stream.seek(-512, 2)
        if stream.read(4) != b"koly":
            raise ValueError("invalid macOS DMG trailer")
    hashes = {name: sha256(directory / name) for name in sorted(RELEASE_ASSETS)}
    print(json.dumps({"version": version, "sha256": hashes}, sort_keys=True))
    return hashes

def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--directory", required=True, type=Path)
    parser.add_argument("--version", required=True)
    args = parser.parse_args()
    verify_directory(args.directory, args.version)
    return 0

if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, struct.error) as error:
        raise SystemExit(f"release verification failed: {error}")
