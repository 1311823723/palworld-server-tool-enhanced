import sys
import unittest
from pathlib import Path
from unittest.mock import patch


sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from snapshot import build_snapshot


GUILD = "10000000-0000-0000-0000-000000000001"
BASE_A = "20000000-0000-0000-0000-000000000001"
BASE_B = "20000000-0000-0000-0000-000000000002"
WORKERS_A = "30000000-0000-0000-0000-000000000001"
WORKERS_B = "30000000-0000-0000-0000-000000000002"
PAL_A = "40000000-0000-0000-0000-000000000001"
PAL_B = "40000000-0000-0000-0000-000000000002"
FARM_A = "60000000-0000-0000-0000-000000000001"
FARM_B = "60000000-0000-0000-0000-000000000002"
FAKE_FARM = "60000000-0000-0000-0000-000000000003"
FARM_C = "60000000-0000-0000-0000-000000000004"
EGG_A = "70000000-0000-0000-0000-000000000001"
NEARBY_EGG = "70000000-0000-0000-0000-000000000002"
CAKE_A = "80000000-0000-0000-0000-000000000001"
CAKE_B = "80000000-0000-0000-0000-000000000002"
CAKE_C = "80000000-0000-0000-0000-000000000003"
FEED = "50000000-0000-0000-0000-000000000001"
CHEST = "50000000-0000-0000-0000-000000000002"
PLAYER_BAG = "50000000-0000-0000-0000-000000000003"


def prop(value):
    return {"value": value}


def enum(value):
    return {"value": {"type": "Enum", "value": value}}


def pal_entry(instance_id, character_id):
    return {
        "key": {"InstanceId": prop(instance_id)},
        "value": {
            "RawData": {
                "value": {
                    "object": {
                        "SaveParameter": {
                            "value": {
                                "CharacterID": prop(character_id),
                                "NickName": prop(character_id),
                                "Level": prop({"value": 20}),
                                "Gender": enum("EPalGenderType::Female"),
                                "Hp": {"value": {"Value": prop(80000)}},
                                "MaxHP": {"value": {"Value": prop(100000)}},
                                "FullStomach": prop(20.0),
                                "SanityValue": prop(75.0),
                                "CraftSpeed": prop({"value": 12}),
                                "OwnerPlayerUId": prop(
                                    "0000002a-0000-0000-0000-000000000000"
                                ),
                            }
                        }
                    }
                }
            }
        },
    }


def character_container(container_id, *instances):
    return {
        "key": {"ID": prop(container_id)},
        "value": {
            "Slots": {
                "value": {
                    "values": [
                        {"RawData": {"value": {"instance_id": instance}}}
                        for instance in instances
                    ]
                }
            }
        },
    }


def item_container(container_id, slots):
    return {
        "key": {"ID": prop(container_id)},
        "value": {
            "Slots": {
                "value": {
                    "values": [
                        {"RawData": {"value": slot}}
                        for slot in slots
                    ]
                }
            }
        },
    }


def item(slot_index, item_id, count):
    return {
        "slot_index": slot_index,
        "count": count,
        "item": {"static_id": item_id},
    }


def base_entry(base_id, worker_container):
    return {
        "key": base_id,
        "value": {
            "RawData": {
                "value": {
                    "id": base_id,
                    "name": f"Base {base_id[-1]}",
                    "group_id_belong_to": GUILD,
                    "area_range": 35.0,
                    "transform": {
                        "translation": {"x": 10, "y": 20, "z": 30}
                    },
                }
            },
            "WorkerDirector": {
                "value": {"RawData": {"value": {"container_id": worker_container}}}
            },
        },
    }


def map_container(base_id, container_id, map_object_id, concrete_type="PalBuildObject"):
    return {
        "MapObjectId": prop(map_object_id),
        "Model": {
            "value": {"RawData": {"value": {"base_camp_id_belong_to": base_id}}}
        },
        "ConcreteModel": {
            "value": {
                "RawData": {"value": {"concrete_model_type": concrete_type}},
                "ModuleMap": {
                    "value": [
                        {
                            "key": "EPalMapObjectConcreteModelModuleType::ItemContainer",
                            "value": {
                                "RawData": {
                                    "value": {"target_container_id": container_id}
                                }
                            },
                        }
                    ]
                },
            }
        },
    }


def breeding_farm(farm_id, base_id, container_id, eggs=None, static_id="BreedFarm"):
    raw = {"concrete_model_type": "PalMapObjectBreedFarmModel"}
    if eggs is not None:
        raw["spawned_egg_instance_ids"] = [prop(egg_id) for egg_id in eggs]
    return {
        "MapObjectInstanceId": prop(farm_id),
        "MapObjectId": prop(static_id),
        "Model": {
            "value": {
                "RawData": {
                    "value": {
                        "instance_id": farm_id,
                        "base_camp_id_belong_to": base_id,
                        "group_id_belong_to": GUILD,
                        "initital_transform_cache": {
                            "translation": {"x": 10, "y": 20, "z": 30}
                        },
                    }
                }
            }
        },
        "ConcreteModel": {
            "value": {
                "RawData": {"value": raw},
                "ModuleMap": {
                    "value": [
                        {
                            "key": "EPalMapObjectConcreteModelModuleType::ItemContainer",
                            "value": {
                                "RawData": {
                                    "value": {"target_container_id": container_id}
                                }
                            },
                        }
                    ]
                },
            }
        },
    }


def egg_object(instance_id):
    return {
        "MapObjectInstanceId": prop(instance_id),
        "MapObjectId": prop("Palegg"),
        "Model": {
            "value": {"RawData": {"value": {"instance_id": instance_id}}}
        },
    }


def breeding_work(farm_id, assignments):
    return {
        "RawData": {
            "value": {
                "owner_map_object_model_id": farm_id,
                "assign_define_data_id": "BreedFarm_0",
            }
        },
        "WorkAssignMap": {
            "value": [
                {
                    "value": {
                        "RawData": {
                            "value": {
                                "location_index": slot,
                                "assigned_individual_id": {"instance_id": pal_id},
                            }
                        }
                    }
                }
                for slot, pal_id in assignments
            ]
        },
    }


def fixture_world():
    return {
        "GroupSaveDataMap": {
            "value": [
                {
                    "key": GUILD,
                    "value": {
                        "GroupType": enum("EPalGroupType::Guild"),
                        "RawData": {
                            "value": {
                                "group_id": GUILD,
                                "guild_name": "Synthetic Guild",
                                "base_camp_level": 12,
                            }
                        },
                    },
                }
            ]
        },
        "BaseCampSaveData": {
            "value": [
                base_entry(BASE_A, WORKERS_A),
                base_entry(BASE_B, WORKERS_B),
            ]
        },
        "CharacterSaveParameterMap": {
            "value": [pal_entry(PAL_A, "SheepBall"), pal_entry(PAL_B, "PinkCat")]
        },
        "CharacterContainerSaveData": {
            "value": [
                character_container(WORKERS_A, PAL_A),
                character_container(WORKERS_B, PAL_B),
            ]
        },
        "MapObjectSaveData": {
            "value": {
                "values": [
                    map_container(
                        BASE_A,
                        FEED,
                        "PalFoodBox",
                        "PalMapObjectPalFoodBoxModel",
                    ),
                    map_container(BASE_A, CHEST, "ItemChest_03"),
                ]
            }
        },
        "ItemContainerSaveData": {
            "value": [
                item_container(
                    FEED,
                    [item(0, "Baked_Berries", 500), item(1, "None", 99)],
                ),
                item_container(CHEST, [item(0, "Stone", 100), item(0, "Stone", 100)]),
                item_container(PLAYER_BAG, [item(0, "Stone", 50), None]),
            ]
        },
    }


def breeding_world():
    world = fixture_world()
    world["MapObjectSaveData"]["value"]["values"].extend(
        [
            breeding_farm(FARM_A, BASE_A, CAKE_A, [EGG_A]),
            breeding_farm(FARM_B, BASE_B, CAKE_B, []),
            breeding_farm(FAKE_FARM, BASE_A, CHEST, [], "MonsterFarm"),
            egg_object(EGG_A),
            egg_object(NEARBY_EGG),
        ]
    )
    world["ItemContainerSaveData"]["value"].extend(
        [
            item_container(CAKE_A, [item(0, "Cake", 4)]),
            item_container(CAKE_B, [item(0, "Cake03", 9)]),
        ]
    )
    # A normal base chest containing cake must not be attributed to either farm.
    world["ItemContainerSaveData"]["value"][1]["value"]["Slots"]["value"][
        "values"
    ].append({"RawData": {"value": item(1, "Cake", 99)}})
    world["WorkSaveData"] = {
        "value": {
            "values": [
                breeding_work(FARM_A, [(0, PAL_A), (1, PAL_B)]),
                breeding_work(FAKE_FARM, [(0, PAL_A)]),
            ]
        }
    }
    return world


def build(world=None):
    return build_snapshot(
        world or fixture_world(),
        [{"player_uid": "42", "nickname": "Synthetic Player"}],
        {
            PLAYER_BAG: {
                "player_uid": "42",
                "source_type": "player_inventory",
                "container_type": "player_inventory",
                "container_name": "CommonContainerId",
            }
        },
        1_700_000_000,
    )


class SnapshotTests(unittest.TestCase):
    def test_base_guild_and_workers_are_not_cross_assigned(self):
        snapshot = build()
        bases = {base["base_id"]: base for base in snapshot["base_camps"]}
        workers = {pal["instance_id"]: pal for pal in snapshot["work_pals"]}
        self.assertEqual(bases[BASE_A]["guild_name"], "Synthetic Guild")
        self.assertEqual(workers[PAL_A]["base_id"], BASE_A)
        self.assertEqual(workers[PAL_B]["base_id"], BASE_B)

    def test_unknown_current_work_is_null_and_capability_is_explicit(self):
        snapshot = build()
        self.assertIsNone(snapshot["work_pals"][0]["current_work"])
        self.assertEqual(snapshot["metadata"]["capabilities"]["current_work"], "partial")

    def test_feed_box_and_regular_storage_are_classified_by_map_object(self):
        snapshot = build()
        containers = {c["container_id"]: c for c in snapshot["containers"]}
        self.assertEqual(containers[FEED]["source_type"], "base_feed_box")
        self.assertEqual(containers[CHEST]["source_type"], "base_storage")

    def test_inventory_deduplicates_locations_and_skips_empty_or_none_slots(self):
        snapshot = build()
        slots = snapshot["inventory_slots"]
        self.assertEqual(len(slots), 3)
        stone = [slot for slot in slots if slot["item_id"] == "stone"]
        self.assertEqual(sum(slot["count"] for slot in stone), 150)
        self.assertEqual(len({slot["location_id"] for slot in slots}), len(slots))

    def test_corrupted_container_is_a_warning_not_a_snapshot_failure(self):
        world = fixture_world()
        world["ItemContainerSaveData"]["value"][0]["value"]["Slots"] = None
        snapshot = build(world)
        self.assertTrue(snapshot["base_camps"])
        self.assertTrue(
            any(
                "item_container_skipped" in warning
                for warning in snapshot["metadata"]["warnings"]
            )
        )

    def test_strict_breeding_links_do_not_cross_farms_or_nearby_objects(self):
        snapshot = build(breeding_world())
        farms = {farm["farm_id"]: farm for farm in snapshot["breeding_farms"]}
        self.assertEqual(set(farms), {FARM_A, FARM_B})
        self.assertEqual(farms[FARM_A]["egg_count"], 1)
        self.assertEqual(farms[FARM_B]["egg_count"], 0)
        self.assertEqual(
            {egg["egg_instance_id"] for egg in snapshot["breeding_eggs"]},
            {EGG_A},
        )
        self.assertNotIn(NEARBY_EGG, {egg["egg_instance_id"] for egg in snapshot["breeding_eggs"]})

    def test_same_base_multiple_farms_remain_separate(self):
        world = breeding_world()
        world["MapObjectSaveData"]["value"]["values"].append(
            breeding_farm(FARM_C, BASE_A, CAKE_C, [])
        )
        world["ItemContainerSaveData"]["value"].append(
            item_container(CAKE_C, [item(0, "Cake02", 17)])
        )
        snapshot = build(world)
        base_a_farms = [farm for farm in snapshot["breeding_farms"] if farm["base_id"] == BASE_A]
        cakes = {cake["farm_id"]: cake["cake_count"] for cake in snapshot["breeding_cakes"]}
        self.assertEqual({farm["farm_id"] for farm in base_a_farms}, {FARM_A, FARM_C})
        self.assertEqual(cakes[FARM_A], 4)
        self.assertEqual(cakes[FARM_C], 17)

    def test_parent_slots_and_cake_boxes_are_owned_by_exact_farm(self):
        snapshot = build(breeding_world())
        parents = [parent for parent in snapshot["breeding_parents"] if parent["farm_id"] == FARM_A]
        cakes = {cake["farm_id"]: cake for cake in snapshot["breeding_cakes"]}
        self.assertEqual({parent["slot_index"] for parent in parents}, {0, 1})
        self.assertEqual(cakes[FARM_A]["cake_count"], 4)
        self.assertEqual(cakes[FARM_B]["cake_count"], 9)
        self.assertNotEqual(cakes[FARM_A]["cake_count"], 103)

    def test_nearby_workers_and_conflicting_parent_assignments_are_not_guessed(self):
        world = breeding_world()
        world["WorkSaveData"]["value"]["values"].append(
            breeding_work(FARM_B, [(0, PAL_A)])
        )
        snapshot = build(world)
        parent_ids = {parent["pal_instance_id"] for parent in snapshot["breeding_parents"]}
        self.assertNotIn(PAL_A, parent_ids)
        farm_b = next(farm for farm in snapshot["breeding_farms"] if farm["farm_id"] == FARM_B)
        self.assertEqual(farm_b["status"], "parent_missing")

    def test_unlinked_box_hatcher_and_wild_eggs_are_not_farm_output(self):
        world = breeding_world()
        world["ItemContainerSaveData"]["value"][1]["value"]["Slots"]["value"][
            "values"
        ].append({"RawData": {"value": item(2, "PalEgg_Normal_01", 6)}})
        world["MapObjectSaveData"]["value"]["values"].append(
            map_container(BASE_A, CHEST, "PalEggHatcher", "PalMapObjectEggHatcherModel")
        )
        snapshot = build(world)
        self.assertEqual(
            {egg["egg_instance_id"] for egg in snapshot["breeding_eggs"]},
            {EGG_A},
        )

    def test_breeding_cake_inventory_has_one_canonical_location(self):
        snapshot = build(breeding_world())
        cake_a = [
            slot
            for slot in snapshot["inventory_slots"]
            if slot["container_id"] == CAKE_A and slot["item_id"] == "cake"
        ]
        self.assertEqual(len(cake_a), 1)
        self.assertEqual(cake_a[0]["source_type"], "breeding_farm_cake_box")

    def test_missing_spawned_egg_field_disables_that_farm_alerts(self):
        world = breeding_world()
        farm = next(
            obj
            for obj in world["MapObjectSaveData"]["value"]["values"]
            if obj.get("MapObjectInstanceId", {}).get("value") == FARM_A
        )
        del farm["ConcreteModel"]["value"]["RawData"]["value"][
            "spawned_egg_instance_ids"
        ]
        snapshot = build(world)
        parsed = next(f for f in snapshot["breeding_farms"] if f["farm_id"] == FARM_A)
        self.assertFalse(parsed["parsing_complete"])
        self.assertIsNone(parsed["egg_count"])

    def test_unvalidated_game_version_is_diagnostic_only(self):
        with patch("breeding.VALIDATED_GAME_VERSION", ""):
            snapshot = build(breeding_world())
        self.assertFalse(snapshot["breeding_capabilities"]["egg_detection"])
        self.assertTrue(all(farm["egg_count"] is None for farm in snapshot["breeding_farms"]))


if __name__ == "__main__":
    unittest.main()
