const exactMessages = {
  "server process is already running or busy": "服务器正在运行或正在处理其他操作",
  "server process is not running": "服务器当前没有运行",
  "server process management is not configured": "尚未配置服务器进程管理",
  "server process management is unsupported on this platform": "当前系统不支持服务器进程管理",
  "invalid server process configuration": "服务器进程配置有误",
};

const fragmentMessages = [
  [/timed out waiting for palserver to exit/i, "等待 PalServer 退出超时"],
  [/timed out waiting for (the )?failed process to exit/i, "等待异常进程退出超时"],
  [/timed out waiting for terminated process to exit/i, "终止进程后等待退出超时"],
  [/palserver health check failed/i, "PalServer 启动检查失败"],
  [/save world/i, "保存世界失败"],
  [/graceful shutdown/i, "平滑关服失败"],
  [/rollback failed/i, "恢复原配置失败"],
  [/previous settings were restored/i, "已恢复修改前的配置"],
  [/unauthorized|invalid token|token is expired/i, "登录已失效，请重新登录"],
  [/conflict/i, "当前有其他操作正在进行，请稍后再试"],
  [/connection refused|failed to fetch|networkerror/i, "无法连接 PST，请确认程序仍在运行"],
  [/context deadline exceeded|timeout/i, "请求超时，请稍后重试"],
];

export function translateBackendMessage(value, fallback = "操作失败，请稍后重试") {
  const raw = String(value || "").trim();
  if (!raw) return fallback;
  const exact = exactMessages[raw.toLowerCase()];
  if (exact) return exact;

  const translated = fragmentMessages
    .filter(([pattern]) => pattern.test(raw))
    .map(([, text]) => text);
  if (translated.length) {
    const details = raw
      .split(/[;；]/)
      .map((part) => {
        const separator = part.indexOf(":");
        const candidate = separator >= 0 ? part.slice(separator + 1).trim() : part.trim();
        return /[\u3400-\u9fff]/.test(candidate) ? candidate : "";
      })
      .filter(Boolean);
    return [...new Set([...translated, ...details])].join("；");
  }

  // 后端已经返回中文时直接保留；英文技术错误不原样甩给服主。
  if (/[\u3400-\u9fff]/.test(raw)) return raw;
  return fallback;
}

export function apiErrorText(data, fallback, statusCode, requestError) {
  const status = Number(statusCode || 0);
  if (status === 401) return "登录已失效，请重新登录";
  if (status === 409) return translateBackendMessage(data?.error, "当前有其他操作正在进行，请稍后再试");
  if (!status && requestError) {
    return /timeout|aborted/i.test(String(requestError))
      ? "连接 PST 超过 8 秒没有响应，请检查服务状态后重试"
      : "无法连接 PST，请检查网络或确认程序仍在运行";
  }
  return translateBackendMessage(data?.error || data?.warning || data?.message, fallback);
}

export function localizeResponseMessages(value, failedResponse = false) {
  if (Array.isArray(value)) return value.map((item) => localizeResponseMessages(item, failedResponse));
  if (!value || typeof value !== "object") return value;
  Object.entries(value).forEach(([key, item]) => {
    if (typeof item === "string" && (
      key === "error"
      || key === "warning"
      || key === "last_error"
      || key.endsWith("_error")
      || (failedResponse && key === "message")
    )) {
      value[key] = translateBackendMessage(
        item,
        key === "warning" ? "服务器返回了一条无法识别的提示" : "操作失败，请稍后重试",
      );
    } else if (item && typeof item === "object") {
      localizeResponseMessages(item, failedResponse);
    }
  });
  return value;
}
