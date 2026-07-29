import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

test("dangerous operations use the selected confirmation levels", async () => {
  const processCard = await readFile(
    new URL("../components/ServerProcessCard.vue", import.meta.url),
    "utf8",
  );
  const shutdownDialog = await readFile(
    new URL("../components/ShutdownDialog.vue", import.meta.url),
    "utf8",
  );
  const worldSettings = await readFile(
    new URL("../views/WorldSettings.vue", import.meta.url),
    "utf8",
  );
  const serverOperations = await readFile(
    new URL("../views/ServerOperations.vue", import.meta.url),
    "utf8",
  );
  const productionOrders = await readFile(
    new URL("../views/ProductionOrders.vue", import.meta.url),
    "utf8",
  );

  assert.doesNotMatch(processCard, /restartForm\.confirmation|stopForm\.confirmation/);
  assert.doesNotMatch(shutdownDialog, /confirmation\.value/);
  assert.match(worldSettings, /confirmText\.value !== "应用"/);
  assert.match(worldSettings, /restoreConfirmText\.value !== "恢复"/);
  assert.match(serverOperations, /confirmation !== "UPDATE"/);
  assert.match(productionOrders, /confirmation !== "INSTALL"/);
});
