export function isWorkerAbnormal(worker) {
  return Boolean(
    worker.status_abnormalities?.length ||
      worker.is_down ||
      worker.is_sick ||
      worker.is_injured ||
      (worker.full_stomach != null && worker.full_stomach < 30) ||
      (worker.sanity != null && worker.sanity < 30) ||
      (worker.hp != null && worker.max_hp > 0 && worker.hp / worker.max_hp < 0.3),
  );
}

export function canAccessAdminRoute(storage) {
  return Boolean(storage?.getItem("palworld_token"));
}

export async function loadOnce(cache, key, loader) {
  if (cache.has(key)) return cache.get(key);
  const pending = Promise.resolve().then(loader);
  cache.set(key, pending);
  try {
    const value = await pending;
    cache.set(key, value);
    return value;
  } catch (error) {
    cache.delete(key);
    throw error;
  }
}

export function breedingCapabilitiesReliable(capabilities) {
  return Boolean(
    capabilities?.farm_detection &&
      capabilities?.base_association &&
      capabilities?.egg_detection &&
      capabilities?.validated_game_version,
  );
}

export function unseenBreedingEvents(events, seenEventIDs) {
  const seen = seenEventIDs instanceof Set ? seenEventIDs : new Set(seenEventIDs || []);
  return (events || []).filter((event) => event?.event_id && !seen.has(event.event_id));
}
