import test from "node:test";
import assert from "node:assert/strict";
import { baseDisplayName, validateBaseAliasName } from "./baseAliases.js";

test("uses custom guild base names and numbered defaults", () => {
  assert.equal(baseDisplayName({ custom_name: "北境矿场", display_name: "据点 1" }, 0), "北境矿场");
  assert.equal(baseDisplayName({ display_name: "据点 2" }, 1), "据点 2");
  assert.equal(baseDisplayName({}, 3), "据点 4");
});

test("validates map base aliases without treating numbered defaults as conflicts", () => {
  const bases = [
    { id: "base-a", display_name: "据点 1" },
    { id: "base-b", display_name: "据点 1", custom_name: "北境矿场" },
  ];
  assert.equal(validateBaseAliasName("第一据点", "base-a", bases), "");
  assert.equal(validateBaseAliasName("北境矿场", "base-a", bases), "当前存档中已有同名据点");
  assert.equal(validateBaseAliasName("含\n换行", "base-a", bases), "名称不能包含换行或控制字符");
});
