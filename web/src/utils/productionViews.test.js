import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

test("production orders route and API methods remain administrator-only surfaces", async () => {
  const router = await readFile(new URL("../router/index.js", import.meta.url), "utf8");
  const api = await readFile(new URL("../service/api.js", import.meta.url), "utf8");
  const view = await readFile(new URL("../views/ProductionOrders.vue", import.meta.url), "utf8");

  assert.match(router, /path:\s*"\/production-orders"[\s\S]*requiresAdmin:\s*true/);
  for (const endpoint of [
    "/api/production/bridge",
    "/api/production/bridge/recheck",
    "/api/production/catalog",
    "/api/production/preview",
    "/api/production/orders",
  ]) {
    assert.ok(api.includes(endpoint), `missing ${endpoint}`);
  }
  assert.match(view, /输入 INSTALL/);
  assert.match(view, /recheckProductionBridge/);
  assert.match(view, /max-width:\s*640px/);
  assert.match(view, /cancellation_requested/);
});
