<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from "vue";
import { useDialog, useMessage } from "naive-ui";
import ApiService from "@/service/api";
import OperationsShell from "@/components/OperationsShell.vue";
import { apiErrorText } from "@/utils/apiError";

const api = new ApiService();
const message = useMessage();
const dialog = useDialog();
const loading = ref(true);
const saving = ref(false);
const testing = ref("");
const status = ref({ enabled: false, connected: false, bot_qq: "", nickname: "", last_error: "" });
const groups = ref([]);
const oneBotToken = ref("");
const deepSeekKey = ref("");
const clearOneBotToken = ref(false);
const clearDeepSeekKey = ref(false);
let statusTimer;
const DEEPSEEK_MODEL = "deepseek-v4-flash";

const form = reactive({
  enabled: false,
  onebot_websocket_url: "ws://127.0.0.1:3001",
  token_is_set: false,
  allowed_group_ids: [],
  admin_qq_ids: [""],
  trigger_mode: "at_only",
  user_rate_per_minute: 5,
  group_rate_per_minute: 30,
  permissions: {
    query_server_status: true,
    query_players: true,
    query_inventory: true,
    query_bases: true,
    query_breeding: true,
    query_backups: true,
    rename_base: true,
    start_server: false,
    restart_server: true,
    stop_server: false,
  },
  notifications: {
    enabled: false,
    group_ids: [],
    server_crash: false,
    watchdog_restart: false,
    scheduled_restart: false,
    backup_failure: false,
    breeding_reminder: false,
  },
  persona: {
    enabled: true,
    style: "lively",
    character: "cattiva",
    serious_on_error: true,
  },
  ai: {
    enabled: false,
    base_url: "https://api.deepseek.com",
    model: DEEPSEEK_MODEL,
    timeout_seconds: 20,
    max_tool_calls: 3,
    send_redacted_results: true,
    api_key_is_set: false,
  },
});

const connectionLabel = computed(() => {
  if (!form.enabled) return "未启用";
  if (status.value.connected) return "已连接";
  return "等待连接";
});
const connectionType = computed(() => status.value.connected ? "success" : form.enabled ? "warning" : "default");
const groupOptions = computed(() => groups.value.map((item) => ({
  label: `${item.group_name || "未命名群"}（${item.group_id}）`,
  value: item.group_id,
})));
const selectedTestGroup = computed(() => form.notifications.group_ids[0] || form.allowed_group_ids[0] || "");
const hasOneBotSecret = computed(() => form.token_is_set || oneBotToken.value.trim());
const hasAISecret = computed(() => form.ai.api_key_is_set || deepSeekKey.value.trim());
const personaStyleOptions = [
  { label: "克制", value: "restrained" },
  { label: "活泼", value: "lively" },
  { label: "调皮", value: "mischievous" },
];
const personaCharacterOptions = [
  { label: "捣蛋喵（Cattiva）", value: "cattiva" },
  { label: "棉悠悠（Lamball）", value: "lamball" },
];
const personaPreview = computed(() => {
  if (!form.persona.enabled) return "服务器当前运行正常。在线玩家：6 人。";
  if (form.persona.character === "lamball") {
    if (form.persona.style === "restrained") return "嗯……棉悠悠查到了，训练家你看一下……服务器当前运行正常，在线玩家 6 人。";
    if (form.persona.style === "mischievous") return "嘿、嘿嘿……虽然有一点点紧张，但棉悠悠还是查到了！服务器当前运行正常，在线玩家 6 人。";
    return "嗯……训、训练家，棉悠悠帮你查到了~服务器当前运行正常，在线玩家 6 人。";
  }
  if (form.persona.style === "restrained") return "好的喵，本喵查到了。服务器当前运行正常，在线玩家 6 人。";
  if (form.persona.style === "mischievous") return "嘿嘿，这点小事可难不倒本喵喵~服务器当前运行正常，在线玩家 6 人。";
  return "喵！本喵帮训练家查到啦~服务器当前运行正常，在线玩家 6 人。";
});

function assignConfig(value) {
  Object.assign(form, value || {});
  form.allowed_group_ids = [...(value?.allowed_group_ids || [])];
  form.admin_qq_ids = (value?.admin_qq_ids?.length ? [...value.admin_qq_ids] : [""]);
  form.permissions = { ...form.permissions, ...(value?.permissions || {}) };
  form.notifications = { ...form.notifications, ...(value?.notifications || {}), group_ids: [...(value?.notifications?.group_ids || [])] };
  form.persona = { ...form.persona, ...(value?.persona || {}) };
  form.ai = { ...form.ai, ...(value?.ai || {}), model: DEEPSEEK_MODEL };
}

async function loadConfig() {
  loading.value = true;
  const { data, statusCode, error } = await api.getQQBotConfig();
  loading.value = false;
  if (statusCode.value !== 200) {
    message.error(apiErrorText(data.value, "QQ 机器人配置读取失败", statusCode.value, error.value));
    return;
  }
  assignConfig(data.value);
  await loadStatus();
  if (status.value.connected) await loadGroups(false);
}

async function loadStatus() {
  const { data, statusCode } = await api.getQQBotStatus();
  if (statusCode.value === 200) status.value = data.value || status.value;
}

async function save() {
  if (saving.value) return;
  const admins = form.admin_qq_ids.map((item) => String(item || "").trim()).filter(Boolean);
  if (form.enabled && !hasOneBotSecret.value) {
    message.warning("启用前请填写 OneBot Token");
    return;
  }
  if (form.ai.enabled && !hasAISecret.value) {
    message.warning("启用 DeepSeek 前请填写 API Key");
    return;
  }
  saving.value = true;
  const payload = {
    enabled: form.enabled,
    onebot_websocket_url: form.onebot_websocket_url,
    onebot_token: oneBotToken.value,
    clear_onebot_token: clearOneBotToken.value,
    allowed_group_ids: [...form.allowed_group_ids],
    admin_qq_ids: admins,
    trigger_mode: "at_only",
    user_rate_per_minute: Number(form.user_rate_per_minute),
    group_rate_per_minute: Number(form.group_rate_per_minute),
    permissions: { ...form.permissions },
    notifications: { ...form.notifications, group_ids: [...form.notifications.group_ids] },
    persona: { ...form.persona },
    ai: {
      enabled: form.ai.enabled,
      base_url: form.ai.base_url,
      api_key: deepSeekKey.value,
      model: DEEPSEEK_MODEL,
      timeout_seconds: Number(form.ai.timeout_seconds),
      max_tool_calls: Number(form.ai.max_tool_calls),
      send_redacted_results: form.ai.send_redacted_results,
    },
    clear_ai_api_key: clearDeepSeekKey.value,
  };
  const { data, statusCode, error } = await api.updateQQBotConfig(payload);
  saving.value = false;
  oneBotToken.value = "";
  deepSeekKey.value = "";
  clearOneBotToken.value = false;
  clearDeepSeekKey.value = false;
  if (statusCode.value !== 200) {
    message.error(apiErrorText(data.value, "保存失败", statusCode.value, error.value));
    return;
  }
  assignConfig(data.value);
  message.success("QQ 机器人配置已保存，连接会自动刷新");
  window.setTimeout(loadStatus, 800);
}

async function testConnection() {
  if (testing.value) return;
  if (!hasOneBotSecret.value) {
    message.warning("请先填写或保存 OneBot Token");
    return;
  }
  testing.value = "connection";
  const { data, statusCode, error } = await api.testQQBotConnection({
    onebot_websocket_url: form.onebot_websocket_url,
    onebot_token: oneBotToken.value,
  });
  testing.value = "";
  if (statusCode.value !== 200) message.error(apiErrorText(data.value, "连接测试失败", statusCode.value, error.value));
  else {
    message.success(`已连接 ${data.value.nickname || "QQ 机器人"}（${data.value.bot_qq}），延迟 ${data.value.latency_ms} ms`);
    await loadStatus();
  }
}

async function reconnect() {
  testing.value = "reconnect";
  const { data, statusCode, error } = await api.reconnectQQBot();
  testing.value = "";
  if (statusCode.value >= 300) message.error(apiErrorText(data.value, "重新连接失败", statusCode.value, error.value));
  else {
    message.info("已开始重新连接 NapCat");
    window.setTimeout(loadStatus, 1000);
  }
}

async function loadGroups(showNotice = true) {
  testing.value = "groups";
  const { data, statusCode, error } = await api.getQQBotGroups();
  testing.value = "";
  if (statusCode.value !== 200) {
    if (showNotice) message.error(apiErrorText(data.value, "群列表读取失败", statusCode.value, error.value));
    return;
  }
  groups.value = data.value?.items || [];
  if (showNotice) message.success(`读取到 ${groups.value.length} 个群`);
}

async function sendTestMessage() {
  if (!selectedTestGroup.value) {
    message.warning("请先选择一个允许群");
    return;
  }
  testing.value = "message";
  const { data, statusCode, error } = await api.testQQBotMessage({ group_id: selectedTestGroup.value });
  testing.value = "";
  if (statusCode.value !== 200) message.error(apiErrorText(data.value, "测试消息发送失败", statusCode.value, error.value));
  else message.success("测试消息已发送");
}

async function testAI() {
  if (!hasAISecret.value) {
    message.warning("请先填写或保存 DeepSeek API Key");
    return;
  }
  testing.value = "ai";
  const { data, statusCode, error } = await api.testQQBotAI({
    api_key: deepSeekKey.value,
    base_url: form.ai.base_url,
    model: DEEPSEEK_MODEL,
    timeout_seconds: Number(form.ai.timeout_seconds),
  });
  testing.value = "";
  if (statusCode.value !== 200) message.error(apiErrorText(data.value, "DeepSeek 测试失败", statusCode.value, error.value));
  else message.success(data.value?.message || "DeepSeek 连接正常");
}

function generateToken() {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  oneBotToken.value = Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("");
  clearOneBotToken.value = false;
}

function requestSecretClear(type) {
  const isOneBot = type === "onebot";
  dialog.warning({
    title: isOneBot ? "清除 OneBot Token" : "清除 DeepSeek API Key",
    content: isOneBot ? "清除后机器人会被自动停用，NapCat 中的 Token 不会改变。" : "清除后 DeepSeek 会被停用，基础命令不受影响。",
    positiveText: "确认清除",
    negativeText: "取消",
    onPositiveClick: () => {
      if (isOneBot) {
        clearOneBotToken.value = true;
        oneBotToken.value = "";
        form.enabled = false;
      } else {
        clearDeepSeekKey.value = true;
        deepSeekKey.value = "";
        form.ai.enabled = false;
      }
    },
  });
}

onMounted(() => {
  loadConfig();
  statusTimer = window.setInterval(loadStatus, 5000);
});
onBeforeUnmount(() => window.clearInterval(statusTimer));
</script>

<template>
  <operations-shell
    title="QQ 机器人"
    subtitle="连接同一台电脑上的 NapCatQQ，让群成员查询服务器信息，管理员可在二次确认后执行指定操作。"
    :loading="loading"
    @refresh="loadConfig"
  >
    <n-skeleton v-if="loading" text :repeat="8" />
    <div v-else class="qq-page">
      <section class="status-panel">
        <div>
          <span class="eyebrow">当前状态</span>
          <div class="status-title">
            <h2>{{ connectionLabel }}</h2>
            <n-tag :type="connectionType" size="small" round>{{ connectionLabel }}</n-tag>
          </div>
          <p v-if="status.connected">{{ status.nickname || "QQ 机器人" }} · {{ status.bot_qq }} · 延迟 {{ status.latency_ms || 0 }} ms</p>
          <p v-else>{{ status.last_error || "保存配置并启动 NapCat 后，PST 会自动连接。" }}</p>
        </div>
        <div class="status-actions">
          <n-button secondary :loading="testing === 'reconnect'" :disabled="!form.enabled" @click="reconnect">重新连接</n-button>
          <n-button type="primary" :loading="testing === 'message'" :disabled="!status.connected || !selectedTestGroup" @click="sendTestMessage">发送测试消息</n-button>
        </div>
      </section>

      <n-alert v-if="status.last_error && !status.connected" type="warning" title="当前问题">
        {{ status.last_error }}
      </n-alert>

      <div class="content-grid">
        <div class="main-column">
          <n-card title="1. 连接 NapCat" size="small">
            <ol class="setup-list">
              <li><b>安装并登录 NapCatQQ</b><span>机器人账号需要能正常进入目标 QQ 群。</span></li>
              <li><b>新建正向 WebSocket 服务端</b><span>在 NapCat“网络配置”中监听 <code>127.0.0.1:3001</code>。</span></li>
              <li><b>设置相同 Token</b><span>这里使用 OneBot Token，不是 NapCat WebUI 的登录 Token。</span></li>
            </ol>
            <n-divider />
            <n-form label-placement="top">
              <div class="form-grid">
                <n-form-item label="OneBot WebSocket 地址">
                  <n-input v-model:value="form.onebot_websocket_url" placeholder="ws://127.0.0.1:3001" />
                </n-form-item>
                <n-form-item :label="form.token_is_set ? 'OneBot Token（已保存）' : 'OneBot Token'">
                  <n-input-group>
                    <n-input v-model:value="oneBotToken" type="password" show-password-on="click" :placeholder="form.token_is_set ? '留空则保留原 Token' : '请设置高强度随机 Token'" @update:value="clearOneBotToken = false" />
                    <n-button @click="generateToken">随机生成</n-button>
                  </n-input-group>
                </n-form-item>
              </div>
              <div class="inline-actions">
                <n-switch v-model:value="form.enabled" />
                <span>启用 QQ 机器人</span>
                <n-button secondary :loading="testing === 'connection'" @click="testConnection">测试连接</n-button>
                <n-button v-if="form.token_is_set" text type="error" @click="requestSecretClear('onebot')">清除已保存 Token</n-button>
              </div>
            </n-form>
          </n-card>

          <n-card title="2. 群聊与管理员" size="small">
            <template #header-extra><n-button text type="primary" :loading="testing === 'groups'" :disabled="!status.connected" @click="loadGroups()">读取群列表</n-button></template>
            <div class="form-grid">
              <n-form-item label="允许使用的群">
                <n-select v-model:value="form.allowed_group_ids" multiple filterable tag :options="groupOptions" placeholder="选择群，或直接输入群号" />
              </n-form-item>
              <n-form-item label="通知发送到">
                <n-select v-model:value="form.notifications.group_ids" multiple :options="groupOptions.filter((item) => form.allowed_group_ids.includes(item.value))" placeholder="从允许群中选择" />
              </n-form-item>
            </div>
            <n-form-item label="管理员 QQ">
              <n-dynamic-input v-model:value="form.admin_qq_ids" :min="1" placeholder="输入 QQ 号" />
            </n-form-item>
            <p class="field-note">群聊只响应 @机器人。只有这里明确填写的 QQ 才能改名或控制 PalServer，不会自动信任群主和群管理员。</p>
          </n-card>

          <n-card title="3. 权限" size="small">
            <div class="permission-groups">
              <div>
                <h3>群成员查询</h3>
                <label><span>服务器状态</span><n-switch v-model:value="form.permissions.query_server_status" /></label>
                <label><span>玩家与在线时间</span><n-switch v-model:value="form.permissions.query_players" /></label>
                <label><span>库存数量与位置</span><n-switch v-model:value="form.permissions.query_inventory" /></label>
                <label><span>据点与工作帕鲁</span><n-switch v-model:value="form.permissions.query_bases" /></label>
                <label><span>配种提醒</span><n-switch v-model:value="form.permissions.query_breeding" /></label>
                <label><span>备份记录</span><n-switch v-model:value="form.permissions.query_backups" /></label>
              </div>
              <div>
                <h3>管理员操作</h3>
                <label><span>修改据点名称</span><n-switch v-model:value="form.permissions.rename_base" /></label>
                <label><span>启动 PalServer</span><n-switch v-model:value="form.permissions.start_server" /></label>
                <label><span>平滑重启 <small>默认开启</small></span><n-switch v-model:value="form.permissions.restart_server" /></label>
                <label><span>平滑停服并保持关闭</span><n-switch v-model:value="form.permissions.stop_server" /></label>
              </div>
            </div>
            <n-alert type="info" :show-icon="false">“关机”只会平滑停止 PalServer，机器人不能关闭 Windows、PST，也不能执行 RCON、CMD、PowerShell 或任意文件操作。</n-alert>
          </n-card>

          <n-card title="4. 主动通知" size="small">
            <div class="section-toggle"><div><b>向指定群发送服务器事件</b><span>默认关闭；断线期间最多暂存 100 条，过期消息不会补发。</span></div><n-switch v-model:value="form.notifications.enabled" /></div>
            <div class="switch-grid" :class="{ disabled: !form.notifications.enabled }">
              <label><span>服务器意外退出</span><n-switch v-model:value="form.notifications.server_crash" :disabled="!form.notifications.enabled" /></label>
              <label><span>守护自动重启</span><n-switch v-model:value="form.notifications.watchdog_restart" :disabled="!form.notifications.enabled" /></label>
              <label><span>计划重启开始</span><n-switch v-model:value="form.notifications.scheduled_restart" :disabled="!form.notifications.enabled" /></label>
              <label><span>备份失败</span><n-switch v-model:value="form.notifications.backup_failure" :disabled="!form.notifications.enabled" /></label>
              <label><span>配种产蛋提醒</span><n-switch v-model:value="form.notifications.breeding_reminder" :disabled="!form.notifications.enabled" /></label>
            </div>
          </n-card>

          <n-card title="5. 回复风格" size="small">
            <div class="section-toggle"><div><b>使用帕鲁人设</b><span>基础命令、主动通知和 DeepSeek 共用；不会改写数量、时间、状态和确认码。</span></div><n-switch v-model:value="form.persona.enabled" /></div>
            <n-collapse-transition :show="form.persona.enabled">
              <div class="form-grid persona-form">
                <n-form-item label="角色">
                  <n-select v-model:value="form.persona.character" :options="personaCharacterOptions" />
                </n-form-item>
                <n-form-item label="语气">
                  <n-select v-model:value="form.persona.style" :options="personaStyleOptions" />
                </n-form-item>
                <n-form-item label="严重故障">
                  <div class="setting-row">
                    <span>发生崩溃、备份失败或操作失败时使用严肃语气</span>
                    <n-switch v-model:value="form.persona.serious_on_error" />
                  </div>
                </n-form-item>
              </div>
              <div class="persona-preview"><span>回复预览</span><p>{{ personaPreview }}</p></div>
            </n-collapse-transition>
          </n-card>

          <n-card title="6. DeepSeek（可选）" size="small">
            <div class="section-toggle"><div><b>让机器人理解更自由的说法</b><span>关闭或调用失败时，固定命令和常见中文问法仍然可用。</span></div><n-switch v-model:value="form.ai.enabled" /></div>
            <n-collapse-transition :show="form.ai.enabled">
              <div class="form-grid ai-form">
                <n-form-item label="模型"><n-input value="DeepSeek V4 Flash" disabled /></n-form-item>
                <n-form-item :label="form.ai.api_key_is_set ? 'API Key（已保存）' : 'API Key'"><n-input v-model:value="deepSeekKey" type="password" show-password-on="click" :placeholder="form.ai.api_key_is_set ? '留空则保留原 Key' : 'sk-…'" @update:value="clearDeepSeekKey = false" /></n-form-item>
                <n-form-item label="超时（秒）"><n-input-number v-model:value="form.ai.timeout_seconds" :min="1" :max="120" /></n-form-item>
                <n-form-item label="单次最多工具调用"><n-input-number v-model:value="form.ai.max_tool_calls" :min="1" :max="5" /></n-form-item>
              </div>
              <div class="inline-actions">
                <n-button secondary :loading="testing === 'ai'" @click="testAI">测试 DeepSeek</n-button>
                <n-button v-if="form.ai.api_key_is_set" text type="error" @click="requestSecretClear('ai')">清除已保存 Key</n-button>
              </div>
              <p class="field-note">仅 DeepSeek API Key 会发送到官方接口。发送前会移除 IP、Steam/User ID、本机路径和技术错误；OneBot Token、JWT、密码不会进入 AI 请求。</p>
            </n-collapse-transition>
          </n-card>
        </div>

        <aside class="help-column">
          <n-card title="基础命令" size="small">
            <code>@机器人 服务器状态</code>
            <code>@机器人 现在谁在线</code>
            <code>@机器人 查询张三在线时间</code>
            <code>@机器人 石头还有多少</code>
            <code>@机器人 第一据点异常帕鲁</code>
            <code>@机器人 最近一次备份</code>
          </n-card>
          <n-card title="管理员命令" size="small">
            <code>@机器人 把旧基地改名为第一据点</code>
            <code>@机器人 重启服务器</code>
            <p>机器人会返回六位验证码。必须由同一 QQ 在同一会话中于 60 秒内确认。</p>
          </n-card>
          <n-card title="连接检查" size="small">
            <ul class="check-list">
              <li :class="{ done: form.onebot_websocket_url.startsWith('ws://127.0.0.1') || form.onebot_websocket_url.startsWith('ws://[::1]') }">只连接本机地址</li>
              <li :class="{ done: hasOneBotSecret }">OneBot Token 已设置</li>
              <li :class="{ done: form.allowed_group_ids.length }">允许群已选择</li>
              <li :class="{ done: form.admin_qq_ids.some(Boolean) }">管理员 QQ 已填写</li>
            </ul>
          </n-card>
        </aside>
      </div>

      <div class="save-bar">
        <div><b>配置保存在本机 config.db</b><span>Token 和 API Key 保存后不再显示。</span></div>
        <n-button type="primary" size="large" :loading="saving" @click="save">保存配置</n-button>
      </div>
    </div>
  </operations-shell>
</template>

<style scoped>
.qq-page { display: grid; gap: 14px; padding-bottom: 74px; }
.status-panel { display: flex; align-items: center; justify-content: space-between; gap: 24px; padding: 22px 24px; border: 1px solid rgba(23,141,121,.2); border-left: 4px solid #178d79; border-radius: 13px; background: rgba(255,255,255,.92); }
.eyebrow { color: #178d79; font-size: 11px; font-weight: 700; letter-spacing: .08em; }
.status-title { display: flex; align-items: center; gap: 10px; margin-top: 4px; }
.status-title h2 { margin: 0; font-size: 24px; letter-spacing: -.03em; }
.status-panel p { margin: 6px 0 0; color: #6c7b74; }
.status-actions, .inline-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
.content-grid { display: grid; grid-template-columns: minmax(0, 1fr) 300px; gap: 14px; align-items: start; }
.main-column, .help-column { display: grid; gap: 14px; min-width: 0; }
.help-column { position: sticky; top: 14px; }
.setup-list { display: grid; gap: 12px; margin: 0; padding-left: 22px; }
.setup-list li { padding-left: 5px; }
.setup-list b, .setup-list span { display: block; }
.setup-list span, .field-note, .section-toggle span, .help-column p { margin-top: 3px; color: #6c7b74; font-size: 12px; line-height: 1.6; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 14px; }
.permission-groups { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; margin-bottom: 14px; }
.permission-groups > div { padding: 14px; border: 1px solid rgba(38,74,61,.13); border-radius: 10px; background: #f8fbf9; }
.permission-groups h3 { margin: 0 0 8px; font-size: 14px; }
.permission-groups label, .switch-grid label { display: flex; align-items: center; justify-content: space-between; gap: 16px; min-height: 38px; border-bottom: 1px solid rgba(38,74,61,.08); font-size: 13px; }
.permission-groups label:last-child { border-bottom: 0; }
.permission-groups small { color: #178d79; font-size: 10px; }
.section-toggle { display: flex; align-items: center; justify-content: space-between; gap: 24px; margin-bottom: 12px; }
.section-toggle b, .section-toggle span { display: block; }
.switch-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 20px; }
.switch-grid.disabled { opacity: .58; }
.ai-form { margin-top: 14px; }
.persona-form { margin-top: 14px; }
.setting-row { display: flex; width: 100%; align-items: center; justify-content: space-between; gap: 16px; color: #55655e; font-size: 12px; line-height: 1.5; }
.persona-preview { padding: 12px 14px; border: 1px solid rgba(23,141,121,.16); border-radius: 9px; background: #f2f8f5; }
.persona-preview span { color: #178d79; font-size: 11px; font-weight: 700; }
.persona-preview p { margin: 5px 0 0; color: #35463f; line-height: 1.65; }
.help-column code { display: block; margin-bottom: 8px; padding: 9px 10px; border-radius: 7px; color: #176b5d; background: #edf6f2; font-size: 12px; white-space: normal; }
.check-list { display: grid; gap: 9px; margin: 0; padding: 0; list-style: none; }
.check-list li { color: #7e8c85; font-size: 13px; }
.check-list li::before { content: "○"; margin-right: 8px; }
.check-list li.done { color: #178d79; }
.check-list li.done::before { content: "●"; }
.save-bar { position: fixed; z-index: 12; right: clamp(20px, 4vw, 52px); bottom: 18px; left: calc(228px + clamp(20px, 4vw, 52px)); display: flex; align-items: center; justify-content: space-between; gap: 20px; max-width: 1316px; margin: 0 auto; padding: 12px 14px 12px 18px; border: 1px solid rgba(23,141,121,.22); border-radius: 12px; background: rgba(255,255,255,.96); box-shadow: 0 14px 36px rgba(30,64,51,.14); backdrop-filter: blur(16px); }
.save-bar b, .save-bar span { display: block; }
.save-bar span { margin-top: 2px; color: #6c7b74; font-size: 11px; }
@media (prefers-color-scheme: dark) {
  .status-panel, .save-bar { background: rgba(24,34,29,.96); }
  .permission-groups > div { background: rgba(255,255,255,.025); }
  .help-column code { color: #82cdbb; background: rgba(23,141,121,.13); }
  .persona-preview { background: rgba(23,141,121,.08); }
  .persona-preview p, .setting-row { color: #c9d5cf; }
}
@media (max-width: 1080px) { .content-grid { grid-template-columns: 1fr; } .help-column { position: static; grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (max-width: 860px) {
  .qq-page { padding-bottom: 90px; }
  .status-panel { align-items: flex-start; padding: 17px; }
  .status-actions { justify-content: flex-end; }
  .help-column { grid-template-columns: 1fr; }
  .save-bar { right: 12px; bottom: 76px; left: 12px; }
}
@media (max-width: 620px) {
  .status-panel { display: grid; gap: 14px; }
  .status-actions { justify-content: flex-start; }
  .form-grid, .permission-groups, .switch-grid { grid-template-columns: 1fr; }
  .section-toggle { align-items: flex-start; }
  .save-bar div { display: none; }
  .save-bar :deep(.n-button) { width: 100%; }
}
</style>
