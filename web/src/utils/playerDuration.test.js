import test from "node:test";
import assert from "node:assert/strict";
import {
  currentSessionSeconds,
  formatOnlineDuration,
  isPlayerCurrentlyOnline,
  totalOnlineSeconds,
} from "./playerDuration.js";

test("prefers the authoritative online flag over last-online heuristics", () => {
  assert.equal(isPlayerCurrentlyOnline({ is_online: false, last_online: new Date().toISOString() }), false);
  assert.equal(isPlayerCurrentlyOnline({ is_online: true }), true);
});

test("calculates current and total online duration from the API snapshot", () => {
  const now = Date.parse("2026-07-22T04:02:00Z");
  const snapshotAt = now - 5000;
  const player = {
    is_online: true,
    online_since: "2026-07-22T04:00:00Z",
    total_online_seconds: 3600,
  };
  assert.equal(currentSessionSeconds(player, now), 120);
  assert.equal(totalOnlineSeconds(player, now, snapshotAt), 3605);
});

test("formats durations as concise Chinese text", () => {
  assert.equal(formatOnlineDuration(9), "9秒");
  assert.equal(formatOnlineDuration(125), "2分钟 5秒");
  assert.equal(formatOnlineDuration(7380), "2小时 3分钟");
  assert.equal(formatOnlineDuration(93780), "1天 2小时 3分钟");
});
