import test from "node:test";
import assert from "node:assert/strict";
import { breedingCapabilitiesReliable, canAccessAdminRoute, isWorkerAbnormal, loadOnce, unseenBreedingEvents } from "./enhancedViews.js";

test("classifies work Pal health without turning unavailable values into zero", () => {
  assert.equal(isWorkerAbnormal({ hp: null, max_hp: null, full_stomach: null, sanity: null }), false);
  assert.equal(isWorkerAbnormal({ hp: 20, max_hp: 100 }), true);
  assert.equal(isWorkerAbnormal({ full_stomach: 29 }), true);
  assert.equal(isWorkerAbnormal({ sanity: 29 }), true);
  assert.equal(isWorkerAbnormal({ status_abnormalities: ["sick"] }), true);
});

test("admin routes require the persisted JWT token", () => {
  assert.equal(canAccessAdminRoute({ getItem: () => null }), false);
  assert.equal(canAccessAdminRoute({ getItem: () => "jwt" }), true);
});

test("lazy inventory details load once and retry after failure", async () => {
  const cache = new Map();
  let calls = 0;
  const loader = async () => { calls += 1; return ["location"]; };
  assert.deepEqual(await loadOnce(cache, "wood", loader), ["location"]);
  assert.deepEqual(await loadOnce(cache, "wood", loader), ["location"]);
  assert.equal(calls, 1);

  await assert.rejects(loadOnce(cache, "stone", async () => { throw new Error("temporary"); }));
  assert.deepEqual(await loadOnce(cache, "stone", loader), ["location"]);
});

test("breeding alerts require explicit validated capabilities", () => {
  assert.equal(breedingCapabilitiesReliable({ farm_detection: true, base_association: true, egg_detection: true, validated_game_version: "Palworld 1.0" }), true);
  assert.equal(breedingCapabilitiesReliable({ farm_detection: true, base_association: true, egg_detection: false, validated_game_version: "Palworld 1.0" }), false);
  assert.equal(breedingCapabilitiesReliable({ farm_detection: true, base_association: true, egg_detection: true, validated_game_version: "" }), false);
});

test("breeding notification IDs are deduplicated across refreshes", () => {
  const events = [{ event_id: "event-a" }, { event_id: "event-b" }, { event_id: "event-a" }];
  assert.deepEqual(unseenBreedingEvents(events, new Set(["event-a"])), [{ event_id: "event-b" }]);
});
