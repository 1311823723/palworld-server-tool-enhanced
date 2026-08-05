import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const source = (relativePath) => readFileSync(
  fileURLToPath(new URL(relativePath, import.meta.url)),
  "utf8",
);

test("QQ bot page keeps secrets ephemeral and explains no-AI commands", () => {
  const page = source("../views/QQBot.vue");
  assert.match(page, /DeepSeek（可选）/);
  assert.match(page, /关闭或调用失败时，固定命令和常见中文问法仍然可用/);
  assert.match(page, /使用帕鲁人设/);
  assert.match(page, /捣蛋喵（Cattiva）/);
  assert.match(page, /棉悠悠（Lamball）/);
  assert.match(page, /严重故障/);
  assert.match(page, /deepseek-v4-flash/);
  assert.doesNotMatch(page, /deepseek-chat|deepseek-reasoner/);
  assert.match(page, /persona: \{ \.\.\.form\.persona \}/);
  assert.match(page, /oneBotToken\.value = ""/);
  assert.match(page, /deepSeekKey\.value = ""/);
  assert.doesNotMatch(page, /localStorage.*oneBot|localStorage.*deepSeek/i);
});

test("QQ bot UI exposes only the fixed administration capabilities", () => {
  const page = source("../views/QQBot.vue");
  ["修改据点名称", "启动 PalServer", "平滑重启", "平滑停服并保持关闭"].forEach((label) => {
    assert.match(page, new RegExp(label));
  });
  assert.match(page, /不能关闭 Windows、PST/);
  assert.doesNotMatch(page, /执行任意命令/);
});

test("QQ bot API methods never put a token in the URL", () => {
  const api = source("../service/api.js");
  assert.match(api, /\/api\/qq-bot\/test-connection/);
  assert.doesNotMatch(api, /qq-bot[^`]*\?.*(token|api_key)/i);
});
