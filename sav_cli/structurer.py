# SPDX-License-Identifier: Apache-2.0
# Derived from zaigie/palworld-server-tool sav_cli @ fb45624 (Apache-2.0).
# Runtime deps (palsav-flex/palooz/ooz) are GPL-3.0-or-later, so a Docker image
# built from the root Dockerfile includes these runtime components.
"""Decode a Palworld 1.0 save and structure it into player / guild JSON.

It uses the ``palsav`` parser from PalworldSaveTools, which ships Palworld 1.0
mappings (GroupSaveDataMap / character / item-container decoders) plus Oodle
(``PlM1``) decompression via the native ``palooz`` module.

The parser performs a full decode. A ~260KB compressed / ~4MB decompressed
Level.sav completes in a couple of seconds on the validated fixtures.
"""

import os

from palsav.core import decompress_sav_to_gvas
from palsav.gvas import GvasFile
from palsav.paltypes import PALWORLD_TYPE_HINTS, PALWORLD_CUSTOM_PROPERTIES

from world_types import Player, Pal, Guild, BaseCamp, hexuid_to_decimal
from logger import log

# Global state shared by the current decode helpers.
wsd = None
gvas_file = None
player_container_owners = {}

PLAYER_CONTAINER_KEYS = [
    "CommonContainerId",
    "DropSlotContainerId",
    "EssentialContainerId",
    "FoodEquipContainerId",
    "PlayerEquipArmorContainerId",
    "WeaponLoadOutContainerId",
]


def _read_gvas(path):
    with open(path, "rb") as f:
        raw_gvas, _ = decompress_sav_to_gvas(f.read())
    return GvasFile.read(raw_gvas, PALWORLD_TYPE_HINTS, PALWORLD_CUSTOM_PROPERTIES)


def convert_sav(file):
    """Decode Level.sav into the module-global ``wsd`` (worldSaveData)."""
    global gvas_file, wsd
    gvas_file = _read_gvas(file)
    wsd = gvas_file.properties["worldSaveData"]["value"]
    return wsd


def _save_parameter(character_entry):
    return character_entry["value"]["RawData"]["value"]["object"]["SaveParameter"][
        "value"
    ]


def structure_player(dir_path, filetime: int = -1):
    global player_container_owners
    player_container_owners = {}
    if not wsd.get("CharacterSaveParameterMap"):
        return [], 0

    ticks = wsd["GameTimeSaveData"]["value"]["RealDateTimeTicks"]["value"]
    item_containers = _index_item_containers()

    players = []
    pals = []
    player_save_warnings = 0
    for c in wsd["CharacterSaveParameterMap"]["value"]:
        uid = c["key"]["PlayerUId"]["value"]
        sp = _save_parameter(c)
        if sp.get("IsPlayer") and sp["IsPlayer"]["value"]:
            sp["Items"], has_warning, sp["Progress"] = getPlayerItems(uid, dir_path, item_containers)
            player_save_warnings += int(has_warning)
            players.append(Player(uid, sp).to_dict())
        else:
            if not sp.get("OwnerPlayerUId"):
                continue
            pals.append(Pal(sp, ticks, filetime).to_dict())

    # De-dup players by uid, keeping the highest-level record.
    unique_players_dict = {}
    for player in players:
        pid = player["player_uid"]
        if pid not in unique_players_dict or player["level"] > unique_players_dict[pid]["level"]:
            unique_players_dict[pid] = player
    unique_players = list(unique_players_dict.values())

    for pal in pals:
        for player in unique_players:
            if player["player_uid"] == pal["owner"]:
                pal.pop("owner")
                player["pals"].append(pal)
                break

    return (
        sorted(unique_players, key=lambda p: p["level"], reverse=True),
        player_save_warnings,
    )


def _index_item_containers():
    """Map container-UUID string -> decoded slots list."""
    index = {}
    if not wsd.get("ItemContainerSaveData"):
        return index
    for container in wsd["ItemContainerSaveData"]["value"]:
        cid = str(container["key"]["ID"]["value"])
        index[cid] = container["value"]["Slots"]["value"]["values"]
    return index


def getPlayerItems(player_uid, dir_path, item_containers):
    containers_data = {k: [] for k in PLAYER_CONTAINER_KEYS}

    player_sav_file = os.path.join(
        dir_path, str(player_uid).upper().replace("-", "") + ".sav"
    )
    if not os.path.exists(player_sav_file):
        return containers_data, True, empty_player_progress()

    try:
        player_gvas = _read_gvas(player_sav_file).properties["SaveData"]["value"]
    except Exception as e:
        log(
            f"Skipped corrupted player save: {os.path.basename(player_sav_file)}: "
            f"{type(e).__name__}: {e}",
            "WARNING",
        )
        return containers_data, True, empty_player_progress()

    progress = extract_player_progress(player_gvas)

    inv = player_gvas.get("InventoryInfo")
    if inv is None:
        return containers_data, False, progress

    for key in PLAYER_CONTAINER_KEYS:
        ref = inv["value"].get(key)
        if ref is None:
            continue
        container_id = str(ref["value"]["ID"]["value"])
        from snapshot import PLAYER_SOURCE_TYPES

        player_container_owners[container_id.lower()] = {
            "player_uid": hexuid_to_decimal(player_uid),
            "source_type": PLAYER_SOURCE_TYPES[key],
            "container_type": PLAYER_SOURCE_TYPES[key],
            "container_name": key,
        }
        slots = item_containers.get(container_id)
        if slots is None:
            continue
        items = []
        for slot in slots:
            raw = slot["RawData"]["value"]
            if not raw:  # empty slot decodes to None
                continue
            static_id = raw["item"]["static_id"]
            if not static_id or static_id.lower() == "none":
                continue
            items.append(
                {
                    "SlotIndex": raw["slot_index"],
                    "ItemId": static_id.lower(),
                    "StackCount": raw["count"],
                }
            )
        containers_data[key] = items
    return containers_data, False, progress


def _property_value(node, default=None):
    if node is None:
        return default
    value = node.get("value", node) if isinstance(node, dict) else node
    while isinstance(value, dict) and set(value.keys()) == {"value"}:
        value = value["value"]
    return value


def _collection_values(node):
    value = _property_value(node, [])
    if isinstance(value, dict):
        value = value.get("values", [])
    return value if isinstance(value, list) else []


def _map_rows(node):
    return _collection_values(node)


def _optional_int(container, key):
    if not isinstance(container, dict) or key not in container:
        return None
    value = _property_value(container.get(key))
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


def _truthy_map_count(node):
    if node is None:
        return None
    count = 0
    for row in _map_rows(node):
        value = _property_value(row.get("value")) if isinstance(row, dict) else None
        if bool(value):
            count += 1
    return count


def _summed_map_count(node):
    if node is None:
        return None
    total = 0
    for row in _map_rows(node):
        value = _property_value(row.get("value")) if isinstance(row, dict) else None
        try:
            total += int(value or 0)
        except (TypeError, ValueError):
            continue
    return total


def empty_player_progress():
    fields = (
        "discovered_pals",
        "captured_pals",
        "fast_travel_points",
        "explored_areas",
        "field_bosses",
        "tower_bosses",
        "dungeons",
        "oil_rig_clears",
        "technology_points",
        "ancient_technology_points",
        "recipes",
    )
    return {
        **{field: None for field in fields},
        "capabilities": {field: False for field in fields},
    }


def extract_player_progress(player_save):
    progress = empty_player_progress()
    if not isinstance(player_save, dict):
        return progress
    record = _property_value(player_save.get("RecordData"), {})
    if not isinstance(record, dict):
        record = {}
    sources = {
        "discovered_pals": (record, "PaldeckUnlockFlag", _truthy_map_count),
        "captured_pals": (record, "PalCaptureCount", _summed_map_count),
        "fast_travel_points": (record, "FastTravelPointUnlockFlag", _truthy_map_count),
        "explored_areas": (record, "FindAreaFlagMap", _truthy_map_count),
        "field_bosses": (record, "NormalBossDefeatFlag", _truthy_map_count),
        "tower_bosses": (record, "TowerBossDefeatFlag", _truthy_map_count),
    }
    for field, (container, key, parser) in sources.items():
        if key in container:
            progress[field] = parser(container.get(key))
            progress["capabilities"][field] = progress[field] is not None
    integer_sources = {
        "dungeons": (record, "FixedDungeonClearCount"),
        "oil_rig_clears": (record, "OilrigClearCount"),
        "technology_points": (player_save, "TechnologyPoint"),
        "ancient_technology_points": (player_save, "bossTechnologyPoint"),
    }
    for field, (container, key) in integer_sources.items():
        value = _optional_int(container, key)
        if value is not None:
            progress[field] = value
            progress["capabilities"][field] = True
    if "UnlockedRecipeTechnologyNames" in player_save:
        progress["recipes"] = len(_collection_values(player_save.get("UnlockedRecipeTechnologyNames")))
        progress["capabilities"]["recipes"] = True
    return progress


def structure_base_camp():
    if not wsd.get("BaseCampSaveData"):
        return []
    return [
        BaseCamp(b["value"]["RawData"]["value"]).to_dict()
        for b in wsd["BaseCampSaveData"]["value"]
    ]


def structure_guild(filetime: int = -1):
    if not wsd.get("GroupSaveDataMap"):
        return []
    base_camps = structure_base_camp()
    ticks = wsd["GameTimeSaveData"]["value"]["RealDateTimeTicks"]["value"]
    groups = (
        g["value"]["RawData"]["value"]
        for g in wsd["GroupSaveDataMap"]["value"]
        if g["value"]["GroupType"]["value"]["value"] == "EPalGroupType::Guild"
    )
    sorted_guilds = sorted(
        (Guild(g, ticks, filetime).to_dict() for g in groups),
        key=lambda g: g["base_camp_level"],
        reverse=True,
    )
    for guild in sorted_guilds:
        for camp in base_camps:
            if camp["id"] in guild["base_ids"]:
                guild["base_camp"].append(
                    {
                        "id": camp["id"],
                        "area": camp["area_range"],
                        "location_x": camp["transform"]["x"],
                        "location_y": camp["transform"]["y"],
                    }
                )
    return list(sorted_guilds)
