#!/usr/bin/env python3
"""Unit tests for the local and GitHub release asset verification scripts."""

from __future__ import annotations

import hashlib
import importlib.util
import tempfile
import unittest
from pathlib import Path
from types import ModuleType


SCRIPTS = Path(__file__).resolve().parent


def load_script(filename: str, module_name: str) -> ModuleType:
    spec = importlib.util.spec_from_file_location(module_name, SCRIPTS / filename)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {filename}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


ASSET_VERIFY = load_script("verify-release-assets.py", "verify_release_assets")
REMOTE_VERIFY = load_script("verify-github-release.py", "verify_github_release")


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


class ReleaseFixture:
    def __init__(self, root: Path) -> None:
        self.root = root

    def create(self) -> Path:
        contents = {
            "ProxyStart-windows-amd64.zip": b"windows package",
            "ProxyStart-source.zip": b"source package",
            "ProxyStart-macos-arm64.zip": b"mac arm package",
            "ProxyStart-macos-amd64.zip": b"mac intel package",
        }
        for name, value in contents.items():
            (self.root / name).write_bytes(value)

        windows = [
            "ProxyStart-windows-amd64.zip",
            "ProxyStart-source.zip",
        ]
        macos = [
            "ProxyStart-macos-arm64.zip",
            "ProxyStart-macos-amd64.zip",
        ]
        (self.root / "ProxyStart-windows.checksums.sha256").write_text(
            "".join(f"{digest(self.root / name)}  {name}\n" for name in windows),
            encoding="ascii",
            newline="\n",
        )
        (self.root / "ProxyStart-macos.checksums.sha256").write_text(
            "".join(f"{digest(self.root / name)}  {name}\n" for name in macos),
            encoding="ascii",
            newline="\n",
        )
        return self.root


class ReleaseAssetVerificationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="proxy-start-release-test-")
        self.root = Path(self.temp.name)
        ReleaseFixture(self.root).create()

    def tearDown(self) -> None:
        self.temp.cleanup()

    def test_exact_assets_and_manifest_hashes(self) -> None:
        aggregate = ASSET_VERIFY.verify_directory(self.root, "0.1.0")
        self.assertTrue(aggregate.is_file())
        self.assertNotIn(b"\r\n", aggregate.read_bytes())
        entries = ASSET_VERIFY.read_manifest(aggregate)
        self.assertEqual(set(entries), ASSET_VERIFY.RELEASE_ASSETS)
        for name, expected in entries.items():
            self.assertEqual(expected, digest(self.root / name))

    def test_existing_aggregate_bytes_are_unchanged(self) -> None:
        aggregate = ASSET_VERIFY.verify_directory(self.root, "0.1.0")
        # CRLF is a valid manifest representation. The verifier must preserve
        # it byte-for-byte once the entries have been validated.
        original = aggregate.read_bytes().replace(b"\n", b"\r\n")
        aggregate.write_bytes(original)
        before = aggregate.read_bytes()

        ASSET_VERIFY.verify_directory(self.root, "0.1.0")

        self.assertEqual(aggregate.read_bytes(), before)

    def test_tampered_missing_and_extra_assets_are_rejected(self) -> None:
        self.root.joinpath("ProxyStart-source.zip").write_bytes(b"tampered")
        with self.assertRaisesRegex(ValueError, "hash mismatch"):
            ASSET_VERIFY.verify_directory(self.root, "0.1.0")

        self.root.joinpath("ProxyStart-source.zip").unlink()
        with self.assertRaisesRegex(ValueError, "release asset set"):
            ASSET_VERIFY.verify_directory(self.root, "0.1.0")

        self.root.joinpath("ProxyStart-source.zip").write_bytes(b"source package")
        self.root.joinpath("unexpected.txt").write_bytes(b"extra")
        with self.assertRaisesRegex(ValueError, "release asset set"):
            ASSET_VERIFY.verify_directory(self.root, "0.1.0")


class RemoteReleaseVerificationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="proxy-start-remote-test-")
        self.root = Path(self.temp.name)
        ReleaseFixture(self.root).create()
        ASSET_VERIFY.verify_directory(self.root, "0.1.0")

    def tearDown(self) -> None:
        self.temp.cleanup()

    def release_payload(self) -> dict:
        assets = []
        for path in sorted(self.root.iterdir()):
            if path.is_file():
                assets.append(
                    {
                        "name": path.name,
                        "size": path.stat().st_size,
                        "digest": f"sha256:{digest(path)}",
                        "url": "https://example.invalid/assets/" + path.name,
                    }
                )
        return {"assets": assets}

    def test_remote_size_and_digest_match(self) -> None:
        REMOTE_VERIFY.verify_release(self.root, self.release_payload())

    def test_remote_size_mismatch_is_rejected(self) -> None:
        release = self.release_payload()
        release["assets"][0]["size"] += 1
        with self.assertRaisesRegex(RuntimeError, "size mismatch"):
            REMOTE_VERIFY.verify_release(self.root, release)

    def test_remote_digest_mismatch_is_rejected(self) -> None:
        release = self.release_payload()
        release["assets"][0]["digest"] = "sha256:" + ("0" * 64)
        with self.assertRaisesRegex(RuntimeError, "digest mismatch"):
            REMOTE_VERIFY.verify_release(self.root, release)


if __name__ == "__main__":
    unittest.main()
