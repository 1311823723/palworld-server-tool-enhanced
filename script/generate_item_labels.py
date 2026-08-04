#!/usr/bin/env python3
"""Generate the compact item-name table embedded by the Go backend."""

from __future__ import annotations

import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "web" / "src" / "assets" / "items.json"
TARGET = ROOT / "internal" / "gamelabels" / "items.json"


def main() -> None:
    source = json.loads(SOURCE.read_text(encoding="utf-8"))
    localized = {
        locale: {str(item["id"]).lower(): item for item in source[locale]}
        for locale in ("en", "zh")
    }
    items = []
    for item_id in sorted(localized["en"]):
        english = localized["en"][item_id]
        chinese = localized["zh"].get(item_id, {})
        items.append(
            {
                "id": item_id,
                "key": english.get("key") or chinese.get("key") or item_id,
                "zh": chinese.get("name") or english.get("name") or item_id,
                "en": english.get("name") or item_id,
            }
        )
    TARGET.write_text(
        json.dumps(items, ensure_ascii=False, separators=(",", ":")) + "\n",
        encoding="utf-8",
    )
    print(f"generated {TARGET.relative_to(ROOT)} ({len(items)} items)")


if __name__ == "__main__":
    main()
