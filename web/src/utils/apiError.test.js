import test from "node:test";
import assert from "node:assert/strict";
import {
  apiErrorText,
  localizeResponseMessages,
  translateBackendMessage,
} from "./apiError.js";

test("maps known backend errors to readable Chinese", () => {
  assert.equal(
    translateBackendMessage("server process is already running or busy"),
    "服务器正在运行或正在处理其他操作",
  );
  assert.equal(
    translateBackendMessage("PalServer health check failed: context deadline exceeded"),
    "PalServer 启动检查失败；请求超时，请稍后重试",
  );
});

test("keeps Chinese errors and hides unknown English implementation details", () => {
  assert.equal(translateBackendMessage("配置文件不存在"), "配置文件不存在");
  assert.equal(translateBackendMessage("some internal library exploded", "读取失败"), "读取失败");
});

test("provides timeout and offline fallbacks", () => {
  assert.match(apiErrorText(null, "失败", 0, "AbortError: timeout"), /8 秒/);
  assert.match(apiErrorText(null, "失败", 0, "Failed to fetch"), /无法连接 PST/);
});

test("localizes nested backend warning and error fields before views render them", () => {
  const response = {
    warning: "some internal library exploded",
    items: [{ last_error: "timed out waiting for PalServer to exit" }],
  };
  assert.deepEqual(localizeResponseMessages(response), {
    warning: "服务器返回了一条无法识别的提示",
    items: [{ last_error: "等待 PalServer 退出超时" }],
  });
});
