#!/usr/bin/env python3
"""Verify the exact package set and the platform hash manifests.

The release workflow downloads artifacts into one directory before creating a
draft release. Keeping the verification here makes the same check runnable in
CI and locally without depending on GitHub's release API.
"""

from __future__ import annotations

import argparse
import hashlib
import re
import sys
from pathlib import Path


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def read_manifest(path: Path) -> dict[str, str]:
    entries: dict[str, str] = {}
    for line_number, raw in enumerate(path.read_text(encoding="ascii").splitlines(), 1):
        line = raw.strip()
        if not line:
            continue
        match = re.fullmatch(r"([0-9a-fA-F]{64})  ([^/\\][^/\\]*)", line)
        if match is None:
            raise ValueError(f"invalid checksum line {path}:{line_number}")
        digest, name = match.groups()
        if name in entries:
            raise ValueError(f"duplicate checksum entry {name!r} in {path}")
        entries[name] = digest.lower()
    return entries


def verify_manifest(directory: Path, name: str, expected: set[str]) -> None:
    manifest = directory / name
    if not manifest.is_file():
        raise ValueError(f"missing checksum manifest: {name}")
    entries = read_manifest(manifest)
    if set(entries) != expected:
        raise ValueError(
            f"{name} contains {sorted(entries)}, expected {sorted(expected)}"
        )
    for asset, expected_hash in entries.items():
        asset_path = directory / asset
        if not asset_path.is_file():
            raise ValueError(f"{name} references missing asset: {asset}")
        actual = sha256(asset_path)
        if actual != expected_hash:
            raise ValueError(
                f"hash mismatch for {asset}: expected {expected_hash}, got {actual}"
            )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--directory", required=True, type=Path)
    parser.add_argument("--version", required=True)
    args = parser.parse_args()

    if re.fullmatch(r"\d+\.\d+\.\d+", args.version) is None:
        raise SystemExit("version must be a three-part numeric version")
    directory = args.directory.resolve()
    if not directory.is_dir():
        raise SystemExit(f"release directory does not exist: {directory}")

    windows = {"ProxyStart-windows-amd64.zip", "ProxyStart-source.zip"}
    macos = {"ProxyStart-macos-arm64.zip", "ProxyStart-macos-amd64.zip"}
    expected = windows | macos | {
        "ProxyStart-windows.checksums.sha256",
        "ProxyStart-macos.checksums.sha256",
    }
    aggregate_name = f"ProxyStart-{args.version}.checksums.sha256"
    actual = {
        entry.name
        for entry in directory.iterdir()
        if entry.is_file() and entry.name != aggregate_name
    }
    if actual != expected:
        raise ValueError(f"release asset set is {sorted(actual)}, expected {sorted(expected)}")

    verify_manifest(directory, "ProxyStart-windows.checksums.sha256", windows)
    verify_manifest(directory, "ProxyStart-macos.checksums.sha256", macos)

    aggregate = directory / aggregate_name
    # Include both archives and the two platform manifests in the top-level
    # release hash so every uploaded asset has an independently verifiable
    # digest (the aggregate file itself is intentionally excluded).
    lines = [f"{sha256(directory / name)}  {name}" for name in sorted(expected)]
    if aggregate.is_file():
        existing = read_manifest(aggregate)
        expected_existing = {
            name: sha256(directory / name) for name in sorted(expected)
        }
        if existing != expected_existing:
            raise ValueError(f"existing aggregate manifest is stale: {aggregate.name}")
    aggregate.write_text("\n".join(lines) + "\n", encoding="ascii")
    print(f"verified {len(expected)} release assets and wrote {aggregate.name}")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except ValueError as error:
        print(f"release verification failed: {error}", file=sys.stderr)
        raise SystemExit(1)
