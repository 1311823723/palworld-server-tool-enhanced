# SPDX-License-Identifier: Apache-2.0
"""Strict, reference-only breeding farm extraction.

No coordinate or nearby-object heuristics are used.  Capabilities stay false
until the exact parser signature has been validated against a current,
privacy-safe save fixture.
"""

from __future__ import annotations


ZERO_UUID = "00000000-0000-0000-0000-000000000000"
BREED_FARM_STATIC_ID = "BreedFarm"
BREED_FARM_CONCRETE_TYPE = "PalMapObjectBreedFarmModel"
ITEM_CONTAINER_MODULE = "EPalMapObjectConcreteModelModuleType::ItemContainer"
CAKE_ITEM_IDS = {"Cake", "Cake02", "Cake03", "Cake04", "Cake05"}

# Updated only after a current-version save has demonstrated the complete
# MapObject -> base/guild -> work slots -> cake container -> spawned egg chain.
VALIDATED_GAME_VERSION = "Palworld 1.0 save fixture (2026-07-22)"


def _value(prop, default=None):
    value = prop
    for _ in range(5):
        if not isinstance(value, dict) or "value" not in value:
            break
        candidate = value["value"]
        if isinstance(candidate, dict) and not set(candidate).issubset(
            {"type", "value", "id"}
        ):
            return candidate
        value = candidate
    return default if value is None else value


def _uuid(value):
    value = _value(value, "")
    return str(value or "").lower()


def _real_uuid(value):
    value = _uuid(value)
    return value if value and value != ZERO_UUID else ""


def _enum(value):
    value = _value(value)
    return None if value is None else str(value).split("::")[-1]


def _integer(value, default=0):
    value = _value(value)
    if isinstance(value, dict):
        value = _value(value)
    try:
        return int(value)
    except (TypeError, ValueError):
        return default


def _fixed_point(value):
    try:
        return int(value["value"]["Value"]["value"])
    except (KeyError, TypeError, ValueError):
        return None


def _number(value):
    value = _value(value)
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def _map_objects(world):
    value = world.get("MapObjectSaveData", {}).get("value", {})
    return value.get("values", []) if isinstance(value, dict) else []


def _model_raw(obj):
    return (
        obj.get("Model", {})
        .get("value", {})
        .get("RawData", {})
        .get("value", {})
        or {}
    )


def _concrete(obj):
    return obj.get("ConcreteModel", {}).get("value", {}) or {}


def _farm_id(obj):
    return _real_uuid(obj.get("MapObjectInstanceId")) or _real_uuid(
        _model_raw(obj).get("instance_id")
    )


def _location(obj, model_raw):
    translation = model_raw.get("initital_transform_cache", {}).get(
        "translation", {}
    )
    if not translation:
        translation = _value(obj.get("WorldLocation"), {}) or {}
    try:
        return {
            "x": float(translation.get("x", 0)),
            "y": float(translation.get("y", 0)),
            "z": float(translation.get("z", 0)),
        }
    except (AttributeError, TypeError, ValueError):
        return {"x": 0.0, "y": 0.0, "z": 0.0}


def _strict_farms(world, bases, guilds, warnings):
    base_by_id = {base["base_id"]: base for base in bases}
    farms = []
    farm_objects = {}
    candidate_count = 0
    for obj in _map_objects(world):
        if str(_value(obj.get("MapObjectId"), "")) != BREED_FARM_STATIC_ID:
            continue
        candidate_count += 1
        model_raw = _model_raw(obj)
        concrete = _concrete(obj)
        concrete_raw = concrete.get("RawData", {}).get("value", {}) or {}
        if concrete_raw.get("concrete_model_type") != BREED_FARM_CONCRETE_TYPE:
            warnings.append("breeding_farm_concrete_model_unverified")
            continue
        farm_id = _farm_id(obj)
        base_id = _real_uuid(model_raw.get("base_camp_id_belong_to"))
        guild_id = _real_uuid(model_raw.get("group_id_belong_to"))
        base = base_by_id.get(base_id)
        if not farm_id or not base or base.get("guild_id") != guild_id:
            warnings.append("breeding_farm_base_reference_invalid")
            continue
        farm = {
            "farm_id": farm_id,
            "base_id": base_id,
            "base_name": base.get("base_name", base_id),
            "guild_id": guild_id,
            "guild_name": guilds.get(guild_id, {}).get("guild_name", ""),
            "map_object_instance_id": farm_id,
            "location": _location(obj, model_raw),
            "status": "unsupported",
            "progress": None,
            "cake_count": None,
            "egg_count": None,
            "confidence": "high" if VALIDATED_GAME_VERSION else "diagnostic",
            "association_verified": True,
            "parsing_complete": bool(VALIDATED_GAME_VERSION),
            "game_version_supported": bool(VALIDATED_GAME_VERSION),
            "identity_supported": False,
            "warnings": [],
            "last_egg_at": None,
        }
        farms.append(farm)
        farm_objects[farm_id] = (obj, concrete_raw)
    return farms, farm_objects, candidate_count


def _pal_from_entry(entry):
    try:
        return entry["value"]["RawData"]["value"]["object"]["SaveParameter"][
            "value"
        ]
    except (KeyError, TypeError):
        return None


def _pal_index(world):
    result = {}
    for entry in world.get("CharacterSaveParameterMap", {}).get("value", []):
        try:
            instance_id = _real_uuid(entry["key"]["InstanceId"])
        except (KeyError, TypeError):
            continue
        value = _pal_from_entry(entry)
        if instance_id and value:
            result[instance_id] = value
    return result


def _parents(world, farm_ids, warnings):
    pals = _pal_index(world)
    candidates = []
    work_entries = world.get("WorkSaveData", {}).get("value", {}).get("values", [])
    for entry in work_entries:
        raw = entry.get("RawData", {}).get("value", {}) or {}
        farm_id = _real_uuid(raw.get("owner_map_object_model_id"))
        if farm_id not in farm_ids:
            continue
        if raw.get("assign_define_data_id") != "BreedFarm_0":
            warnings.append("breeding_parent_work_definition_unverified")
            continue
        seen = set()
        for assignment in entry.get("WorkAssignMap", {}).get("value", []):
            assignment_raw = (
                assignment.get("value", {})
                .get("RawData", {})
                .get("value", {})
                or {}
            )
            instance_id = _real_uuid(
                assignment_raw.get("assigned_individual_id", {}).get("instance_id")
            )
            slot = _integer(assignment_raw.get("location_index"), -1)
            if slot < 0 or slot > 1 or not instance_id or instance_id in seen:
                if instance_id:
                    warnings.append("breeding_parent_assignment_invalid")
                continue
            seen.add(instance_id)
            pal = pals.get(instance_id)
            if pal is None:
                warnings.append("breeding_parent_character_missing")
                continue
            candidates.append(
                {
                    "farm_id": farm_id,
                    "slot_index": slot,
                    "pal_instance_id": instance_id,
                    "pal_id": str(_value(pal.get("CharacterID"), "unknown")),
                    "pal_name": str(_value(pal.get("CharacterID"), "unknown")),
                    "nickname": str(_value(pal.get("NickName"), "")),
                    "gender": _enum(pal.get("Gender")),
                    "level": _integer(pal.get("Level"), 1),
                    "hp": _fixed_point(pal.get("Hp")),
                    "max_hp": _fixed_point(pal.get("MaxHP")),
                    "san": _number(pal.get("SanityValue")),
                    "owner_player_name": "",
                    "assignment_verified": bool(VALIDATED_GAME_VERSION),
                }
            )
    instance_counts = {}
    slot_counts = {}
    for parent in candidates:
        instance_counts[parent["pal_instance_id"]] = (
            instance_counts.get(parent["pal_instance_id"], 0) + 1
        )
        slot_key = (parent["farm_id"], parent["slot_index"])
        slot_counts[slot_key] = slot_counts.get(slot_key, 0) + 1
    result = []
    for parent in candidates:
        slot_key = (parent["farm_id"], parent["slot_index"])
        if instance_counts[parent["pal_instance_id"]] != 1:
            warnings.append("breeding_parent_assigned_to_multiple_farms")
            continue
        if slot_counts[slot_key] != 1:
            warnings.append("breeding_parent_slot_conflict")
            continue
        result.append(parent)
    return result


def _item_containers(world):
    result = {}
    for entry in world.get("ItemContainerSaveData", {}).get("value", []):
        try:
            container_id = _real_uuid(entry["key"]["ID"])
        except (KeyError, TypeError):
            continue
        if container_id:
            result[container_id] = entry
    return result


def _farm_container(concrete):
    matches = []
    for module in concrete.get("ModuleMap", {}).get("value", []):
        if module.get("key") != ITEM_CONTAINER_MODULE:
            continue
        raw = module.get("value", {}).get("RawData", {}).get("value", {}) or {}
        container_id = _real_uuid(raw.get("target_container_id"))
        if container_id:
            matches.append(container_id)
    return matches[0] if len(matches) == 1 else ""


def _slot_values(entry):
    return (
        entry.get("value", {})
        .get("Slots", {})
        .get("value", {})
        .get("values", [])
    )


def _cakes(world, farm_objects, warnings):
    containers = _item_containers(world)
    result = []
    for farm_id, (obj, _) in farm_objects.items():
        container_id = _farm_container(_concrete(obj))
        entry = containers.get(container_id)
        cake_slots = []
        other_items = []
        if entry:
            for slot in _slot_values(entry):
                raw = slot.get("RawData", {}).get("value") or {}
                item_id = str(raw.get("item", {}).get("static_id", ""))
                count = _integer(raw.get("count"), 0)
                slot_index = _integer(raw.get("slot_index"), -1)
                if count <= 0 or slot_index < 0 or not item_id or item_id == "None":
                    continue
                value = {"slot_index": slot_index, "item_id": item_id, "count": count}
                if item_id in CAKE_ITEM_IDS:
                    cake_slots.append(value)
                else:
                    other_items.append(value)
        local_warnings = []
        if not container_id or entry is None:
            local_warnings.append("breeding_cake_container_missing")
        if other_items:
            local_warnings.append("breeding_cake_container_has_non_cake_items")
        warnings.extend(local_warnings)
        count = sum(slot["count"] for slot in cake_slots)
        verified = bool(VALIDATED_GAME_VERSION and container_id and entry is not None)
        result.append(
            {
                "farm_id": farm_id,
                "container_id": container_id,
                "cake_item_id": (
                    cake_slots[0]["item_id"] if verified and cake_slots else None
                ),
                "cake_count": count if verified else None,
                "slots": cake_slots if verified else [],
                "verified": verified,
                "warnings": local_warnings,
            }
        )
    return result


def _eggs(world, farm_objects, warnings):
    object_by_id = {_farm_id(obj): obj for obj in _map_objects(world) if _farm_id(obj)}
    result = []
    incomplete_farms = set()
    for farm_id, (_, concrete_raw) in farm_objects.items():
        spawned = concrete_raw.get("spawned_egg_instance_ids")
        if not isinstance(spawned, list):
            warnings.append("breeding_spawned_egg_ids_unavailable")
            incomplete_farms.add(farm_id)
            continue
        seen = set()
        for value in spawned:
            instance_id = _real_uuid(value)
            if not instance_id or instance_id in seen:
                continue
            seen.add(instance_id)
            egg_object = object_by_id.get(instance_id)
            object_id = str(_value((egg_object or {}).get("MapObjectId"), ""))
            verified = bool(
                VALIDATED_GAME_VERSION
                and egg_object
                and object_id.lower()
                in {
                    "palegg", "palegg_fire", "palegg_water", "palegg_leaf",
                    "palegg_electricity", "palegg_ice", "palegg_earth",
                    "palegg_dark", "palegg_dragon",
                }
            )
            if not verified:
                warnings.append("breeding_output_egg_reference_unverified")
                incomplete_farms.add(farm_id)
            result.append(
                {
                    "farm_id": farm_id,
                    "egg_instance_id": instance_id,
                    "egg_item_id": None,
                    "egg_name": "Unknown egg",
                    "count": 1,
                    "slot_index": None,
                    "ready": True,
                    "association_verified": verified,
                }
            )
    return result, incomplete_farms


def build_breeding_records(world, bases, guilds):
    warnings = []
    farms, farm_objects, candidates = _strict_farms(world, bases, guilds, warnings)
    parents = _parents(world, set(farm_objects), warnings)
    cakes = _cakes(world, farm_objects, warnings)
    eggs, incomplete_egg_farms = _eggs(world, farm_objects, warnings)
    parent_counts = {}
    for parent in parents:
        parent_counts[parent["farm_id"]] = parent_counts.get(parent["farm_id"], 0) + 1
    cake_by_farm = {cake["farm_id"]: cake for cake in cakes}
    eggs_by_farm = {}
    for egg in eggs:
        eggs_by_farm.setdefault(egg["farm_id"], []).append(egg)
    for farm in farms:
        cake = cake_by_farm.get(farm["farm_id"])
        farm_eggs = eggs_by_farm.get(farm["farm_id"], [])
        farm["cake_count"] = cake.get("cake_count") if cake else None
        egg_data_complete = farm["farm_id"] not in incomplete_egg_farms
        farm["parsing_complete"] = farm["parsing_complete"] and egg_data_complete
        if not egg_data_complete:
            farm["warnings"].append("breeding_egg_data_incomplete")
        farm["egg_count"] = (
            sum(egg["count"] for egg in farm_eggs)
            if VALIDATED_GAME_VERSION
            and egg_data_complete
            and all(egg["association_verified"] for egg in farm_eggs)
            else None
        )
        farm["identity_supported"] = bool(
            VALIDATED_GAME_VERSION
            and egg_data_complete
            and all(egg["egg_instance_id"] for egg in farm_eggs)
        )
        parent_count = parent_counts.get(farm["farm_id"], 0)
        if farm["egg_count"] and farm["egg_count"] > 0:
            farm["status"] = "egg_ready"
        elif parent_count < 2:
            farm["status"] = "parent_missing"
        elif farm["cake_count"] == 0:
            farm["status"] = "cake_empty"
        else:
            farm["status"] = "unknown"
        if parent_count < 2:
            farm["warnings"].append("breeding_parent_slots_incomplete")
        if not VALIDATED_GAME_VERSION:
            farm["warnings"].append("breeding_current_game_version_not_validated")
    supported = bool(VALIDATED_GAME_VERSION)
    capabilities = {
        "farm_detection": supported and candidates == len(farms),
        "base_association": supported,
        "parent_slots": supported,
        "cake_container": supported,
        "egg_detection": supported,
        "egg_identity": supported,
        "egg_type": False,
        "breeding_progress": False,
        "validated_game_version": VALIDATED_GAME_VERSION,
    }
    return {
        "farms": farms,
        "parents": parents,
        "cakes": cakes,
        "eggs": eggs,
        "capabilities": capabilities,
        "warnings": sorted(set(warnings)),
    }
