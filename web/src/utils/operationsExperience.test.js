import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const source = (relativePath) => readFileSync(
  fileURLToPath(new URL(relativePath, import.meta.url)),
  "utf8",
);

test("overview keeps the five common server actions on the first screen", () => {
  const dashboard = source("../views/Dashboard.vue");
  const processCard = source("../components/ServerProcessCard.vue");
  ["立即备份", "游戏内广播"].forEach((label) => assert.match(dashboard, new RegExp(label)));
  ["saveWorld", "restartVisible", "stopVisible"].forEach((key) => assert.match(processCard, new RegExp(key)));
});

test("operations page loads only the active tab", () => {
  const page = source("../views/ServerOperations.vue");
  assert.match(page, /async function loadTab/);
  assert.match(page, /onMounted\(\(\) => loadTab\(activeTab\.value\)\)/);
  assert.doesNotMatch(page, /Promise\.all\(\[loadConfig\(\), loadBackups\(\)/);
});

test("world settings uses the new task-oriented structure", () => {
  const worldSettings = source("../views/WorldSettings.vue");
  assert.match(worldSettings, /commonSettingKeys/);
  assert.match(worldSettings, /settings-action-bar/);
});
