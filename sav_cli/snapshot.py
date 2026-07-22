# SPDX-License-Identifier: Apache-2.0
"""Build privacy-safe, normalized read-only indexes from decoded world data.

This module deliberately depends only on plain Python dictionaries so its
relationship logic can be tested with synthetic fixtures without palsav or a
real Palworld save.
"""

from __future__ import annotations

from datetime import datetime, timezone

from breeding import build_breeding_records


ZERO_UUID = "00000000-0000-0000-0000-000000000000"

PLAYER_SOURCE_TYPES = {
    "CommonContainerId": "player_inventory",
    "DropSlotContainerId": "player_drop_slot",
    "EssentialContainerId": "player_essential",
    "FoodEquipContainerId": "player_food",
    "PlayerEquipArmorContainerId": "player_equipment",
    "WeaponLoadOutContainerId": "player_weapon",
}


def _value(prop, default=None):
    """Unwrap scalar palsav properties while leaving structured values intact."""
    if prop is None:
        return default
    value = prop
    for _ in range(4):
        if not isinstance(value, dict) or "value" not in value:
            break
        candidate = value["value"]
        if isinstance(candidate, dict) and not (
            set(candidate).issubset({"type", "value", "id"})
        ):
            return candidate
        value = candidate
    return value


def _uuid(value):
    value = _value(value, "")
    return str(value or "").lower()


def _is_real_uuid(value):
    normalized = _uuid(value)
    return bool(normalized and normalized != ZERO_UUID)


def _fixed_point(prop):
    try:
        return int(prop["value"]["Value"]["value"])
    except (KeyError, TypeError, ValueError):
        return None


def _number(prop):
    value = _value(prop)
    if value is None or isinstance(value, (dict, list)):
        return None
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def _integer(prop, default=0):
    value = _number(prop)
    return default if value is None else int(value)


def _enum_suffix(prop):
    value = _value(prop)
    if value is None or isinstance(value, (dict, list)):
        return None
    return str(value).split("::")[-1]


def _bool(prop):
    value = _value(prop)
    return value if isinstance(value, bool) else None


def _character_save_parameter(entry):
    try:
        return entry["value"]["RawData"]["value"]["object"]["SaveParameter"][
            "value"
        ]
    except (KeyError, TypeError):
        return None


def _entry_instance_id(entry):
    try:
        return _uuid(entry["key"]["InstanceId"])
    except (KeyError, TypeError):
        return ""


def _container_id(entry):
    try:
        return _uuid(entry["key"]["ID"])
    except (KeyError, TypeError):
        return ""


def _base_id(entry):
    try:
        raw = entry["value"]["RawData"]["value"]
        return _uuid(raw.get("id") or entry.get("key"))
    except (KeyError, TypeError):
        return ""


def _guild_id(entry):
    try:
        raw = entry["value"]["RawData"]["value"]
        return _uuid(raw.get("group_id") or entry.get("key"))
    except (KeyError, TypeError):
        return ""


def _transform(raw):
    try:
        translation = raw["transform"]["translation"]
        return {
            "x": float(translation.get("x", 0)),
            "y": float(translation.get("y", 0)),
            "z": float(translation.get("z", 0)),
        }
    except (KeyError, TypeError, ValueError):
        return {"x": 0.0, "y": 0.0, "z": 0.0}


def _classify_base_container(map_object_id, concrete_model_type):
    marker = f"{map_object_id} {concrete_model_type}".lower()
    if "palfoodbox" in marker:
        return "base_feed_box", "feed_box"
    if any(token in marker for token in ("refrigerator", "fridge", "coolerbox")):
        return "base_refrigerator", "refrigerator"
    if any(
        token in marker
        for token in ("itemchest", "guildchest", "storage", "container", "box")
    ):
        return "base_storage", "storage"
    return "unknown_container", "unknown_container"


def _abnormal_state(value, healthy_tokens):
    if value is None:
        return None
    normalized = str(value).split("::")[-1].lower()
    return not any(token in normalized for token in healthy_tokens)


def _work_assignments(world, warnings):
    assignments = {}
    try:
        entries = world.get("WorkSaveData", {}).get("value", {}).get("values", [])
    except AttributeError:
        entries = []
    for entry in entries:
        try:
            work_type = _enum_suffix(entry.get("WorkableType"))
            raw = entry.get("RawData", {}).get("value", {})
            base_id = _uuid(raw.get("base_camp_id_belong_to"))
            target = _uuid(
                raw.get("owner_map_object_model_id")
                or raw.get("owner_map_object_concrete_model_id")
            )
            for assignment in entry.get("WorkAssignMap", {}).get("value", []):
                assignment_raw = (
                    assignment.get("value", {})
                    .get("RawData", {})
                    .get("value", {})
                )
                individual = assignment_raw.get("assigned_individual_id", {})
                instance_id = _uuid(individual.get("instance_id"))
                if not instance_id:
                    continue
                assignments[instance_id] = {
                    "current_work": work_type,
                    "current_work_target": target or None,
                    "work_base_id": base_id or None,
                }
        except (AttributeError, KeyError, TypeError) as error:
            warnings.append(f"work_assignment_skipped:{type(error).__name__}")
    return assignments


def _build_pal_index(world, player_names, warnings):
    work_assignments = _work_assignments(world, warnings)
    pals = {}
    entries = world.get("CharacterSaveParameterMap", {}).get("value", [])
    for entry in entries:
        sp = _character_save_parameter(entry)
        if not sp or _bool(sp.get("IsPlayer")) is True:
            continue
        instance_id = _entry_instance_id(entry)
        if not instance_id:
            warnings.append("pal_without_instance_id")
            continue
        owner_uid = str(sp.get("_pst_owner_uid", ""))
        if not owner_uid:
            owner = _uuid(sp.get("OwnerPlayerUId"))
            if owner:
                try:
                    owner_uid = str(int(owner.split("-")[0], 16))
                except ValueError:
                    owner_uid = owner

        worker_sick = _enum_suffix(sp.get("WorkerSick"))
        physical_health = _enum_suffix(sp.get("PhysicalHealth"))
        hunger_type = _enum_suffix(sp.get("HungerType"))
        status_abnormalities = []
        if _abnormal_state(worker_sick, ("none", "healthy", "normal")) is True:
            status_abnormalities.append(worker_sick)
        if _abnormal_state(physical_health, ("none", "healthy", "normal")) is True:
            status_abnormalities.append(physical_health)
        if _abnormal_state(hunger_type, ("none", "full", "normal")) is True:
            status_abnormalities.append(hunger_type)

        work = work_assignments.get(instance_id, {})
        pal = {
            "instance_id": instance_id,
            "pal_id": str(_value(sp.get("CharacterID"), "unknown")),
            "nickname": str(_value(sp.get("NickName"), "")),
            "level": _integer(sp.get("Level"), 1),
            "gender": _enum_suffix(sp.get("Gender")),
            "hp": _fixed_point(sp.get("Hp")),
            "max_hp": _fixed_point(sp.get("MaxHP")),
            "full_stomach": _number(sp.get("FullStomach")),
            "sanity": _number(sp.get("SanityValue")),
            "is_sleeping": _bool(sp.get("IsSleeping")),
            "is_down": (
                None
                if physical_health is None
                else any(
                    token in physical_health.lower()
                    for token in ("down", "dead", "dying", "incapacitated")
                )
            ),
            "is_injured": _abnormal_state(
                physical_health, ("none", "healthy", "normal")
            ),
            "is_sick": _abnormal_state(
                worker_sick, ("none", "healthy", "normal")
            ),
            "status_abnormalities": status_abnormalities,
            "current_work": work.get("current_work"),
            "current_work_target": work.get("current_work_target"),
            "work_suitability": _enum_suffix(sp.get("CurrentWorkSuitability")),
            "work_speed": _integer(sp.get("CraftSpeed"), 0),
            "owner_player_uid": owner_uid,
            "owner_player_name": player_names.get(owner_uid, ""),
            "data_availability": {
                "hp": sp.get("Hp") is not None,
                "max_hp": sp.get("MaxHP") is not None,
                "full_stomach": sp.get("FullStomach") is not None,
                "sanity": sp.get("SanityValue") is not None,
                "current_work": instance_id in work_assignments,
                "sleeping": sp.get("IsSleeping") is not None,
            },
        }
        pals[instance_id] = pal
    return pals


def _guild_index(world):
    result = {}
    entries = world.get("GroupSaveDataMap", {}).get("value", [])
    for entry in entries:
        try:
            group_type = _enum_suffix(entry["value"].get("GroupType"))
            if group_type != "Guild":
                continue
            raw = entry["value"]["RawData"]["value"]
            guild_id = _guild_id(entry)
            result[guild_id] = {
                "guild_id": guild_id,
                "guild_name": str(raw.get("guild_name", "")),
                "base_camp_level": int(raw.get("base_camp_level", 0)),
            }
        except (KeyError, TypeError, ValueError):
            continue
    return result


def _base_records(world, guilds, pals, warnings):
    character_containers = {
        _container_id(entry): entry
        for entry in world.get("CharacterContainerSaveData", {}).get("value", [])
        if _container_id(entry)
    }
    bases = []
    workers = []
    for entry in world.get("BaseCampSaveData", {}).get("value", []):
        try:
            value = entry["value"]
            raw = value["RawData"]["value"]
            base_id = _base_id(entry)
            guild_id = _uuid(raw.get("group_id_belong_to"))
            guild = guilds.get(guild_id, {})
            worker_container_id = _uuid(
                value.get("WorkerDirector", {})
                .get("value", {})
                .get("RawData", {})
                .get("value", {})
                .get("container_id")
            )
            base = {
                "base_id": base_id,
                "base_name": str(raw.get("name", "")) or base_id,
                "guild_id": guild_id,
                "guild_name": guild.get("guild_name", ""),
                "base_camp_level": guild.get("base_camp_level", 0),
                "location": _transform(raw),
                "area_range": float(raw.get("area_range", 0)),
                "worker_container_id": worker_container_id,
                "container_data_available": bool(worker_container_id),
            }
            bases.append(base)

            container = character_containers.get(worker_container_id)
            if not container:
                if worker_container_id:
                    warnings.append(f"worker_container_missing:{base_id}")
                continue
            slots = (
                container.get("value", {})
                .get("Slots", {})
                .get("value", {})
                .get("values", [])
            )
            seen_instances = set()
            for slot in slots:
                raw_slot = slot.get("RawData", {}).get("value")
                if not raw_slot:
                    continue
                instance_id = _uuid(raw_slot.get("instance_id"))
                if not instance_id or instance_id in seen_instances:
                    continue
                seen_instances.add(instance_id)
                pal = pals.get(instance_id)
                if not pal:
                    warnings.append(f"worker_pal_missing:{base_id}")
                    continue
                worker = dict(pal)
                worker["base_id"] = base_id
                worker["base_name"] = base["base_name"]
                worker["guild_id"] = guild_id
                worker["guild_name"] = base["guild_name"]
                workers.append(worker)
        except (AttributeError, KeyError, TypeError, ValueError) as error:
            warnings.append(f"base_camp_skipped:{type(error).__name__}")
    return bases, workers


def _base_container_records(world, base_by_id, warnings):
    containers = {}
    try:
        map_objects = (
            world.get("MapObjectSaveData", {}).get("value", {}).get("values", [])
        )
    except AttributeError:
        map_objects = []
    for obj in map_objects:
        try:
            model_raw = obj.get("Model", {}).get("value", {}).get("RawData", {}).get(
                "value", {}
            )
            base_id = _uuid(model_raw.get("base_camp_id_belong_to"))
            if base_id not in base_by_id:
                continue
            map_object_id = str(_value(obj.get("MapObjectId"), ""))
            concrete = obj.get("ConcreteModel", {}).get("value", {})
            concrete_raw = concrete.get("RawData", {}).get("value", {}) or {}
            concrete_type = str(concrete_raw.get("concrete_model_type", ""))
            modules = concrete.get("ModuleMap", {}).get("value", [])
            for module in modules:
                if module.get("key") != "EPalMapObjectConcreteModelModuleType::ItemContainer":
                    continue
                module_raw = (
                    module.get("value", {}).get("RawData", {}).get("value", {}) or {}
                )
                container_id = _uuid(module_raw.get("target_container_id"))
                if not _is_real_uuid(container_id):
                    continue
                source_type, container_type = _classify_base_container(
                    map_object_id, concrete_type
                )
                base = base_by_id[base_id]
                candidate = {
                    "container_id": container_id,
                    "source_type": source_type,
                    "container_type": container_type,
                    "container_name": map_object_id or concrete_type or container_type,
                    "base_id": base_id,
                    "base_name": base.get("base_name", ""),
                    "guild_id": base.get("guild_id", ""),
                    "guild_name": base.get("guild_name", ""),
                }
                current = containers.get(container_id)
                if current is None or (
                    current["source_type"] == "unknown_container"
                    and source_type != "unknown_container"
                ):
                    containers[container_id] = candidate
        except (AttributeError, TypeError) as error:
            warnings.append(f"map_container_skipped:{type(error).__name__}")
    return containers


def _all_inventory_records(
    world, base_containers, player_container_owners, player_names, warnings
):
    containers = dict(base_containers)
    for container_id, owner in player_container_owners.items():
        normalized_id = _uuid(container_id)
        if not normalized_id:
            continue
        player_uid = str(owner.get("player_uid", ""))
        containers[normalized_id] = {
            "container_id": normalized_id,
            "source_type": owner.get("source_type", "player_inventory"),
            "container_type": owner.get("container_type", owner.get("source_type", "")),
            "container_name": owner.get("container_name", owner.get("source_type", "")),
            "player_uid": player_uid,
            "player_name": player_names.get(player_uid, ""),
        }

    item_containers = {
        _container_id(entry): entry
        for entry in world.get("ItemContainerSaveData", {}).get("value", [])
        if _container_id(entry)
    }
    slots = []
    seen = set()
    for container_id, container in containers.items():
        entry = item_containers.get(container_id)
        if not entry:
            container["parsed"] = False
            warnings.append(f"item_container_missing:{container_id}")
            continue
        container["parsed"] = True
        try:
            raw_slots = (
                entry.get("value", {})
                .get("Slots", {})
                .get("value", {})
                .get("values", [])
            )
            for slot in raw_slots:
                raw = slot.get("RawData", {}).get("value")
                if not raw:
                    continue
                item_id = str(raw.get("item", {}).get("static_id", "")).lower()
                count = int(raw.get("count", 0))
                slot_index = int(raw.get("slot_index", -1))
                if not item_id or item_id == "none" or count <= 0 or slot_index < 0:
                    continue
                location_id = f"{container_id}:{slot_index}"
                if location_id in seen:
                    continue
                seen.add(location_id)
                record = {
                    "location_id": location_id,
                    "item_id": item_id,
                    "item_name": item_id,
                    "count": count,
                    "slot_index": slot_index,
                    "spoil_remaining_seconds": None,
                    **container,
                }
                record.pop("parsed", None)
                slots.append(record)
        except (AttributeError, TypeError, ValueError) as error:
            container["parsed"] = False
            warnings.append(f"item_container_skipped:{type(error).__name__}")
    return list(containers.values()), slots


def build_snapshot(
    world,
    players,
    player_container_owners,
    save_file_time,
    parser_warnings=0,
):
    """Return a normalized snapshot suitable for transactional BoltDB storage."""
    warnings = []
    player_names = {
        str(player.get("player_uid", "")): str(player.get("nickname", ""))
        for player in players
    }
    guilds = _guild_index(world)
    pals = _build_pal_index(world, player_names, warnings)
    bases, workers = _base_records(world, guilds, pals, warnings)
    base_by_id = {base["base_id"]: base for base in bases}
    base_containers = _base_container_records(world, base_by_id, warnings)
    breeding = build_breeding_records(world, bases, guilds)
    warnings.extend(breeding["warnings"])
    for cake in breeding["cakes"]:
        if not cake["verified"] or not cake["container_id"]:
            continue
        farm = next(
            (value for value in breeding["farms"] if value["farm_id"] == cake["farm_id"]),
            None,
        )
        if farm is None:
            continue
        base_containers[cake["container_id"]] = {
            "container_id": cake["container_id"],
            "source_type": "breeding_farm_cake_box",
            "container_type": "breeding_farm_cake_box",
            "container_name": f"Breeding farm {cake['farm_id'][:8]}",
            "base_id": farm["base_id"],
            "base_name": farm["base_name"],
            "guild_id": farm["guild_id"],
            "guild_name": farm["guild_name"],
        }
    containers, inventory_slots = _all_inventory_records(
        world,
        base_containers,
        player_container_owners,
        player_names,
        warnings,
    )
    if parser_warnings:
        warnings.append(f"player_save_warnings:{int(parser_warnings)}")

    snapshot_time = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    file_time = datetime.fromtimestamp(save_file_time, timezone.utc).isoformat().replace(
        "+00:00", "Z"
    )
    return {
        "metadata": {
            "snapshot_time": snapshot_time,
            "save_file_time": file_time,
            "is_stale": False,
            "warnings": sorted(set(warnings)),
            "capabilities": {
                "base_camps": "available",
                "work_pals": "available",
                "current_work": "partial",
                "sanity": "partial",
                "sleeping": "partial",
                "feed_boxes": "available",
                "inventory": "available",
                "spoil_remaining_seconds": "unavailable",
                "breeding_farms": (
                    "available"
                    if breeding["capabilities"]["farm_detection"]
                    else "unavailable"
                ),
                "breeding_alerts": (
                    "available"
                    if breeding["capabilities"]["egg_detection"]
                    else "unavailable"
                ),
            },
        },
        "base_camps": bases,
        "work_pals": workers,
        "containers": containers,
        "inventory_slots": inventory_slots,
        "breeding_farms": breeding["farms"],
        "breeding_parents": breeding["parents"],
        "breeding_cakes": breeding["cakes"],
        "breeding_eggs": breeding["eggs"],
        "breeding_capabilities": breeding["capabilities"],
    }
