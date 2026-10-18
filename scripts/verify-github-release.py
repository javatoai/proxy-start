#!/usr/bin/env python3
"""Verify that a GitHub Release contains exactly the locally verified assets."""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
import tempfile
from pathlib import Path


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def download_asset(url: str, target: Path) -> None:
    command = [
        "gh",
        "api",
        url,
        "--header",
        "Accept: application/octet-stream",
    ]
    with target.open("wb") as stream:
        result = subprocess.run(command, stdout=stream, stderr=subprocess.PIPE, text=False)
    if result.returncode != 0:
        error = result.stderr.decode("utf-8", errors="replace").strip()
        raise RuntimeError(error or f"gh api failed for {url}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--directory", required=True, type=Path)
    parser.add_argument("--release-json", required=True, type=Path)
    args = parser.parse_args()

    directory = args.directory.resolve()
    release = json.loads(args.release_json.read_text(encoding="utf-8"))
    local = {entry.name: entry for entry in directory.iterdir() if entry.is_file()}
    remote = {asset["name"]: asset for asset in release.get("assets", [])}
    if set(local) != set(remote):
        raise RuntimeError(
            f"remote asset set is {sorted(remote)}, expected {sorted(local)}"
        )

    with tempfile.TemporaryDirectory(prefix="proxy-start-release-verify-") as temp:
        temp_dir = Path(temp)
        for name, local_path in sorted(local.items()):
            asset = remote[name]
            remote_size = asset.get("size")
            if remote_size != local_path.stat().st_size:
                raise RuntimeError(
                    f"size mismatch for {name}: remote {remote_size}, local {local_path.stat().st_size}"
                )
            digest = asset.get("digest") or ""
            if digest.startswith("sha256:"):
                remote_hash = digest.removeprefix("sha256:").lower()
            else:
                downloaded = temp_dir / name
                download_asset(asset["url"], downloaded)
                remote_hash = sha256(downloaded)
            local_hash = sha256(local_path)
            if remote_hash != local_hash:
                raise RuntimeError(
                    f"digest mismatch for {name}: remote {remote_hash}, local {local_hash}"
                )
    print(f"verified GitHub Release asset set and hashes: {len(local)} files")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, RuntimeError, ValueError, KeyError) as error:
        raise SystemExit(f"GitHub Release verification failed: {error}")
