#!/usr/bin/env python3
"""Print a privacy-safe Palworld save capability report.

The report contains only file metadata, hashes, aggregate counts, and known
field-presence booleans. It never prints player names, IDs, item IDs, guild
names, passwords, or raw save values.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "sav_cli"))


def count_values(world, key):
    value = world.get(key, {})
    if not isinstance(value, dict):
        return 0
    value = value.get("value", [])
    if isinstance(value, dict):
        value = value.get("values", [])
    return len(value) if isinstance(value, list) else 0


def main():
    parser = argparse.ArgumentParser(
        description="Inspect a Level.sav without exposing identities or inventory details"
    )
    parser.add_argument("file", type=Path)
    args = parser.parse_args()
    path = args.file.resolve()
    if not path.is_file() or path.name.lower() != "level.sav":
        raise SystemExit("file must be an existing Level.sav")

    from structurer import convert_sav

    world = convert_sav(str(path))
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    report = {
        "format": "pst-save-diagnostic-v1",
        "file": {
            "size_bytes": path.stat().st_size,
            "mtime_unix": int(path.stat().st_mtime),
            "sha256_prefix": digest[:16],
        },
        "counts": {
            "characters": count_values(world, "CharacterSaveParameterMap"),
            "guilds_or_groups": count_values(world, "GroupSaveDataMap"),
            "base_camps": count_values(world, "BaseCampSaveData"),
            "character_containers": count_values(world, "CharacterContainerSaveData"),
            "item_containers": count_values(world, "ItemContainerSaveData"),
            "map_objects": count_values(world, "MapObjectSaveData"),
            "work_records": count_values(world, "WorkSaveData"),
        },
        "capability_fields": {
            key: key in world
            for key in (
                "CharacterSaveParameterMap",
                "GroupSaveDataMap",
                "BaseCampSaveData",
                "CharacterContainerSaveData",
                "ItemContainerSaveData",
                "MapObjectSaveData",
                "WorkSaveData",
            )
        },
        "privacy": "No identities, names, item details, raw values, or secrets included.",
    }
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
