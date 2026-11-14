#!/usr/bin/env python3
"""Unit tests for the local and GitHub release asset verification scripts."""

from __future__ import annotations

import hashlib
import importlib.util
import tempfile
import struct
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
        exe = bytearray(256)
        exe[:2] = b"MZ"
        struct.pack_into("<I", exe, 0x3c, 0x80)
        exe[0x80:0x84] = b"PE\0\0"
        struct.pack_into("<H", exe, 0x84, 0x8664)
        struct.pack_into("<H", exe, 0x80+92, 2)
        (self.root / "ProxyStart-windows-amd64.exe").write_bytes(exe)
        (self.root / "ProxyStart-macos-universal.dmg").write_bytes(bytes(512)+b"koly"+bytes(508))
        return self.root


class ReleaseAssetVerificationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="proxy-start-release-test-")
        self.root = Path(self.temp.name)
        ReleaseFixture(self.root).create()

    def tearDown(self) -> None:
        self.temp.cleanup()

    def test_exact_two_assets_without_extra_manifest(self) -> None:
        hashes = ASSET_VERIFY.verify_directory(self.root, "0.2.0")
        self.assertEqual(set(hashes), ASSET_VERIFY.RELEASE_ASSETS)
        self.assertEqual(set(p.name for p in self.root.iterdir()), ASSET_VERIFY.RELEASE_ASSETS)
        for name, expected in hashes.items():
            self.assertEqual(expected, digest(self.root / name))

    def test_missing_and_extra_files_or_directories_are_rejected(self) -> None:
        asset = self.root / "ProxyStart-windows-amd64.exe"
        original = asset.read_bytes()
        asset.unlink()
        with self.assertRaisesRegex(ValueError, "release asset set"):
            ASSET_VERIFY.verify_directory(self.root, "0.2.0")
        asset.write_bytes(original)
        for name in ["README.md", "ProxyStart-source.zip", "checksums.sha256"]:
            extra = self.root / name
            extra.write_bytes(b"extra")
            with self.assertRaisesRegex(ValueError, "release asset set"):
                ASSET_VERIFY.verify_directory(self.root, "0.2.0")
            extra.unlink()
        (self.root / "docs").mkdir()
        with self.assertRaisesRegex(ValueError, "release asset set"):
            ASSET_VERIFY.verify_directory(self.root, "0.2.0")

    def test_console_executable_and_fake_dmg_are_rejected(self) -> None:
        exe = self.root / "ProxyStart-windows-amd64.exe"
        data = bytearray(exe.read_bytes())
        struct.pack_into("<H", data, 0x80+92, 3)
        exe.write_bytes(data)
        with self.assertRaisesRegex(ValueError, "GUI executable"):
            ASSET_VERIFY.verify_directory(self.root, "0.2.0")
        ReleaseFixture(self.root).create()
        (self.root / "ProxyStart-macos-universal.dmg").write_bytes(b"not a disk image")
        with self.assertRaisesRegex(ValueError, "invalid macOS DMG"):
            ASSET_VERIFY.verify_directory(self.root, "0.2.0")


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
