#!/usr/bin/env python3
"""Verify the bundled PST Production Bridge allowlist and SHA-256 values."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path, PurePosixPath


ROOT = Path(__file__).resolve().parents[1]
BRIDGE = ROOT / "extras" / "PSTProductionBridge"


def verify_bridge(root: Path = BRIDGE) -> None:
    manifest_path = root / "manifest.json"
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    if manifest.get("name") != "PSTProductionBridge":
        raise ValueError("unexpected Bridge manifest name")
    if manifest.get("protocol_version") != 1:
        raise ValueError("unexpected Bridge protocol version")
    allowed = {"manifest.json"}
    for item in manifest.get("files", []):
        relative = PurePosixPath(item["path"])
        if relative.is_absolute() or ".." in relative.parts:
            raise ValueError(f"unsafe Bridge manifest path: {relative}")
        path = root.joinpath(*relative.parts)
        if not path.is_file() or path.is_symlink():
            raise FileNotFoundError(f"Bridge file is missing or not regular: {relative}")
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        if digest.lower() != str(item["sha256"]).lower():
            raise ValueError(f"Bridge SHA-256 mismatch: {relative}")
        allowed.add(relative.as_posix())
    actual = {
        path.relative_to(root).as_posix()
        for path in root.rglob("*")
        if path.is_file()
    }
    unexpected = actual - allowed
    if unexpected:
        raise ValueError(f"Bridge contains files outside manifest: {sorted(unexpected)}")


if __name__ == "__main__":
    verify_bridge()
    print("PST Production Bridge manifest OK")
