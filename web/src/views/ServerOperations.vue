<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import dayjs from "dayjs";
import { useDialog, useMessage } from "naive-ui";
import { useRoute, useRouter } from "vue-router";
import ApiService from "@/service/api";
import OperationsShell from "@/components/OperationsShell.vue";
import ServerProcessCard from "@/components/ServerProcessCard.vue";
import ConfigManager from "@/components/ConfigManager.vue";
import RconManager from "@/components/RconManager.vue";
import { apiErrorText, translateBackendMessage } from "@/utils/apiError";

const api = new ApiService();
const route = useRoute();
const router = useRouter();
const message = useMessage();
const dialog = useDialog();
const allowedTabs = ["process", "schedule", "update", "backup", "rcon", "logs", "audit"];
const activeTab = ref(allowedTabs.includes(String(route.query.tab)) ? String(route.query.tab) : "process");
const loading = ref(false);
const loadedTabs = ref(new Set(["process"]));
const tabError = ref("");
const action = ref("");
const showConfig = ref(false);
const showRcon = ref(false);
const backups = ref([]);
const commands = ref([]);
const tasks = ref([]);
const logs = ref([]);
const audits = ref([]);
const logLevel = ref("");
const logCursor = ref(0);
const configData = ref(null);
const previewTimes = ref([]);
const previewDescription = ref("");
const previewTimezone = ref("");
const previewError = ref("");
const previewLoading = ref(false);
const updateStatus = ref({ enabled: false, checking: false, running: false });
const updateForm = reactive({
  confirmation: "",
  shutdown_seconds: 30,
  restart_delay_seconds: 10,
  message: "服务器将在 30 秒后更新，请提前回到安全位置。",
});
let logTimer;
let previewTimer;

const schedule = computed(() => configData.value?.server_process || {});
const frequencyOptions = [
  { label: "每天", value: "daily" },
  { label: "每隔几天", value: "interval_days" },
  { label: "每周", value: "weekly" },
  { label: "每月", value: "monthly" },
  { label: "高级 Cron", value: "cron" },
];
const weekdayOptions = ["星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"]
  .map((label, value) => ({ label, value }));
const formatBytes = (value) => {
  const bytes = Number(value || 0);
  if (!bytes) return "—";
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
};
const actionLabel = (value) => {
  const labels = {
    "POST /api/server/start": "启动服务器",
    "POST /api/server/restart": "平滑重启",
    "POST /api/server/stop": "平滑停服",
    "POST /api/server/update/apply": "更新服务器",
    "POST /api/backup": "创建备份",
  };
  return labels[value] || value;
};

async function loadConfig() {
  const { data, statusCode, error } = await api.getConfig();
  if (statusCode.value === 200) configData.value = data.value;
  else throw new Error(apiErrorText(data.value, "自动重启配置读取失败", statusCode.value, error.value));
}
async function loadBackups() {
  const { data, statusCode, error } = await api.getBackupList({});
  if (statusCode.value === 200) backups.value = (Array.isArray(data.value) ? data.value : []).reverse();
  else throw new Error(apiErrorText(data.value, "备份记录读取失败", statusCode.value, error.value));
}
async function loadRconSummary() {
  const [commandResponse, taskResponse] = await Promise.all([api.getRconCommands(), api.getRconTasks()]);
  if (commandResponse.statusCode.value === 200) commands.value = commandResponse.data.value || [];
  if (taskResponse.statusCode.value === 200) tasks.value = taskResponse.data.value || [];
  if (commandResponse.statusCode.value !== 200 || taskResponse.statusCode.value !== 200) {
    const failed = commandResponse.statusCode.value !== 200 ? commandResponse : taskResponse;
    throw new Error(apiErrorText(failed.data.value, "RCON 信息读取失败", failed.statusCode.value, failed.error?.value));
  }
}
async function loadUpdate() {
  const { data, statusCode, error } = await api.getServerUpdate();
  if (statusCode.value === 200) updateStatus.value = data.value || updateStatus.value;
  else throw new Error(apiErrorText(data.value, "服务器更新状态读取失败", statusCode.value, error.value));
}
async function loadAudits() {
  const { data, statusCode, error } = await api.getOperationAudits({ limit: 200 });
  if (statusCode.value === 200) audits.value = data.value?.items || [];
  else throw new Error(apiErrorText(data.value, "操作记录读取失败", statusCode.value, error.value));
}
async function loadLogs(reset = false) {
  if (reset) {
    logCursor.value = 0;
    logs.value = [];
  }
  const { data, statusCode, error } = await api.getRuntimeLogs({
    limit: 300,
    after_id: logCursor.value,
    level: logLevel.value,
  });
  if (statusCode.value === 200) {
    const incoming = data.value?.items || [];
    logs.value = [...logs.value, ...incoming].slice(-800);
    logCursor.value = data.value?.next_cursor || logCursor.value;
  } else {
    throw new Error(apiErrorText(data.value, "运行日志读取失败", statusCode.value, error.value));
  }
}
function startLogPolling() {
  clearInterval(logTimer);
  if (activeTab.value !== "logs") return;
  logTimer = window.setInterval(() => loadLogs().catch(() => {}), 2000);
}
async function loadTab(tab = activeTab.value, force = false) {
  if (!force && loadedTabs.value.has(tab)) return;
  loading.value = true;
  tabError.value = "";
  try {
    if (tab === "schedule") {
      await loadConfig();
      await previewSchedule(false);
    } else if (tab === "update") await loadUpdate();
    else if (tab === "backup") await loadBackups();
    else if (tab === "rcon") await loadRconSummary();
    else if (tab === "logs") await loadLogs(true);
    else if (tab === "audit") await loadAudits();
    loadedTabs.value = new Set([...loadedTabs.value, tab]);
  } catch (error) {
    tabError.value = error?.message || "当前内容加载失败，请重试";
  } finally {
    loading.value = false;
  }
}
async function load() {
  await loadTab(activeTab.value, true);
}
async function createBackup() {
  if (action.value) return;
  action.value = "backup";
  const { data, statusCode, error } = await api.createBackup();
  action.value = "";
  if (statusCode.value !== 200) message.error(apiErrorText(data.value, "备份创建失败", statusCode.value, error.value));
  else {
    message.success("存档备份已创建");
    await loadBackups();
  }
}
async function downloadBackup(item) {
  action.value = item.backup_id;
  const { data, execute } = await api.downloadBackup(item.backup_id);
  await execute();
  const url = URL.createObjectURL(data.value);
  const link = document.createElement("a");
  link.href = url;
  link.download = item.path;
  link.click();
  URL.revokeObjectURL(url);
  action.value = "";
}
function removeBackup(item) {
  dialog.warning({
    title: "删除存档备份",
    content: `确定永久删除 ${item.path} 吗？`,
    positiveText: "删除",
    negativeText: "取消",
    onPositiveClick: async () => {
      const { data, statusCode, error } = await api.removeBackup(item.backup_id);
      if (statusCode.value !== 200) message.error(apiErrorText(data.value, "删除失败", statusCode.value, error.value));
      else await loadBackups();
    },
  });
}
async function previewSchedule(notifyOnError = true) {
  const value = schedule.value;
  if (!value.scheduled_restart_frequency) return;
  previewLoading.value = true;
  const { data, statusCode, error } = await api.previewRestartSchedule({
    frequency: value.scheduled_restart_frequency,
    time: value.scheduled_restart_time,
    interval_days: value.scheduled_restart_interval_days,
    start_date: value.scheduled_restart_start_date,
    weekday: value.scheduled_restart_weekday,
    day_of_month: value.scheduled_restart_day_of_month,
    cron_expression: value.cron_expression,
  });
  previewLoading.value = false;
  if (statusCode.value !== 200) {
    previewTimes.value = [];
    previewDescription.value = "";
    previewTimezone.value = "";
    previewError.value = apiErrorText(data.value, "计划表达式无效", statusCode.value, error.value);
    if (notifyOnError) message.error(previewError.value);
  } else {
    previewTimes.value = data.value?.items || [];
    previewDescription.value = data.value?.description || "";
    previewTimezone.value = data.value?.timezone || "";
    previewError.value = "";
  }
}
async function saveSchedule() {
  if (!configData.value || action.value) return;
  action.value = "schedule";
  const { data, statusCode, error } = await api.updateConfig({ settings: configData.value, new_password: "" });
  action.value = "";
  if (statusCode.value !== 200) message.error(apiErrorText(data.value, "自动重启计划保存失败", statusCode.value, error.value));
  else {
    message.success("自动重启计划已保存并立即生效");
    await previewSchedule();
  }
}
async function checkUpdate() {
  action.value = "check-update";
  const { data, statusCode, error } = await api.checkServerUpdate();
  action.value = "";
  updateStatus.value = data.value || updateStatus.value;
  if (statusCode.value !== 200) message.error(apiErrorText(data.value, "检查更新失败", statusCode.value, error.value));
  else if (updateStatus.value.available) message.warning(`发现新版本：Build ${updateStatus.value.latest_build}`);
  else message.success("当前服务器已是最新版本");
}
async function applyUpdate() {
  if (updateForm.confirmation !== "UPDATE" || action.value) return;
  action.value = "apply-update";
  const { data, statusCode, error } = await api.applyServerUpdate(updateForm);
  action.value = "";
  if (statusCode.value !== 200) message.error(apiErrorText(data.value, "服务器更新失败", statusCode.value, error.value));
  else {
    updateForm.confirmation = "";
    message.success("服务器更新流程已完成");
    await loadUpdate();
  }
}

watch(activeTab, (value) => {
  router.replace({ query: { ...route.query, tab: value } });
  loadTab(value);
  startLogPolling();
});
watch(logLevel, () => {
  if (activeTab.value === "logs") loadLogs(true).catch((error) => { tabError.value = error.message; });
});
watch(
  () => [
    schedule.value.scheduled_restart_frequency,
    schedule.value.scheduled_restart_time,
    schedule.value.scheduled_restart_interval_days,
    schedule.value.scheduled_restart_start_date,
    schedule.value.scheduled_restart_weekday,
    schedule.value.scheduled_restart_day_of_month,
    schedule.value.cron_expression,
  ],
  () => {
    clearTimeout(previewTimer);
    if (activeTab.value === "schedule") {
      previewTimer = window.setTimeout(() => previewSchedule(false), 350);
    }
  },
);
onMounted(() => loadTab(activeTab.value));
onBeforeUnmount(() => {
  clearInterval(logTimer);
  clearTimeout(previewTimer);
});
</script>

<template>
  <operations-shell title="服务器运维" subtitle="集中管理 PalServer 进程、自动重启、更新、备份、RCON、日志与操作记录。" :loading="loading" @refresh="load">
    <n-tabs v-model:value="activeTab" type="line" animated class="ops-tabs">
      <n-tab-pane name="process" tab="进程控制" />
      <n-tab-pane name="schedule" tab="自动重启" />
      <n-tab-pane name="update" tab="服务器更新" />
      <n-tab-pane name="backup" tab="存档备份" />
      <n-tab-pane name="rcon" tab="RCON" />
      <n-tab-pane name="logs" tab="PST 日志" />
      <n-tab-pane name="audit" tab="操作审计" />
    </n-tabs>

    <n-alert v-if="tabError" type="error" class="tab-error">
      <div class="retry-row"><span>{{ tabError }}</span><n-button size="small" secondary @click="load">重试</n-button></div>
    </n-alert>
    <div v-if="loading && !loadedTabs.has(activeTab)" class="tab-skeleton">
      <n-skeleton text :repeat="2" />
      <n-skeleton height="220px" />
    </div>

    <server-process-card v-else-if="activeTab === 'process'" :is-admin="true" />

    <section v-else-if="activeTab === 'schedule' && configData" class="tool-panel schedule-panel">
      <header><div><h2>自动重启计划</h2><p>选择执行频率和时间，保存后立即生效。</p></div><n-switch v-model:value="schedule.scheduled_restart_enabled"><template #checked>已开启</template><template #unchecked>已关闭</template></n-switch></header>
      <div class="form-grid">
        <n-form-item label="执行频率"><n-select v-model:value="schedule.scheduled_restart_frequency" :options="frequencyOptions" /></n-form-item>
        <n-form-item v-if="schedule.scheduled_restart_frequency !== 'cron'" label="执行时间"><n-time-picker v-model:formatted-value="schedule.scheduled_restart_time" value-format="HH:mm" format="HH:mm" /></n-form-item>
        <n-form-item v-if="schedule.scheduled_restart_frequency === 'interval_days'" label="间隔天数"><n-input-number v-model:value="schedule.scheduled_restart_interval_days" :min="1" :max="3650" /></n-form-item>
        <n-form-item v-if="schedule.scheduled_restart_frequency === 'interval_days'" label="起始日期"><n-date-picker v-model:formatted-value="schedule.scheduled_restart_start_date" type="date" value-format="yyyy-MM-dd" /></n-form-item>
        <n-form-item v-if="schedule.scheduled_restart_frequency === 'weekly'" label="星期"><n-select v-model:value="schedule.scheduled_restart_weekday" :options="weekdayOptions" /></n-form-item>
        <n-form-item v-if="schedule.scheduled_restart_frequency === 'monthly'" label="每月日期"><n-input-number v-model:value="schedule.scheduled_restart_day_of_month" :min="1" :max="31" /></n-form-item>
        <n-form-item v-if="schedule.scheduled_restart_frequency === 'cron'" label="Cron 表达式" feedback="分钟 小时 日 月 星期；必须正好五段，不支持秒字段"><n-input v-model:value="schedule.cron_expression" placeholder="0 4 * * *" /></n-form-item>
      </div>
      <div class="preview">
        <span>未来三次执行</span>
        <b v-if="previewDescription">{{ previewDescription }}<small v-if="previewTimezone"> · {{ previewTimezone }}</small></b>
        <strong v-for="value in previewTimes" :key="value">{{ dayjs(value).format("YYYY-MM-DD HH:mm:ss") }}</strong>
        <small v-if="previewError" class="preview-error">{{ previewError }}</small>
        <small v-else-if="!previewTimes.length">正在计算未来执行时间</small>
      </div>
      <n-collapse class="learn-more"><n-collapse-item title="了解 Cron 和执行规则" name="schedule-help"><p>高级模式使用标准五段 Cron：分钟、小时、日期、月份、星期。计划触发后仍会先广播和保存，等待旧进程完全退出后再启动。</p></n-collapse-item></n-collapse>
      <footer><n-button secondary @click="showConfig = true">完整进程配置</n-button><n-button :loading="previewLoading" @click="previewSchedule(true)">校验计划</n-button><n-button type="primary" :loading="action === 'schedule'" @click="saveSchedule">保存并生效</n-button></footer>
    </section>

    <section v-else-if="activeTab === 'update'" class="tool-panel update-panel">
      <header><div><h2>服务器更新</h2><p>检查并安装 Palworld Dedicated Server 更新。</p></div><n-tag :type="updateStatus.available ? 'warning' : updateStatus.error ? 'error' : 'success'">{{ updateStatus.available ? "发现更新" : updateStatus.error ? "检查失败" : "暂无更新" }}</n-tag></header>
      <dl class="update-facts"><div><dt>已安装 Build</dt><dd>{{ updateStatus.installed_build || "—" }}</dd></div><div><dt>最新 Build</dt><dd>{{ updateStatus.latest_build || "—" }}</dd></div><div><dt>最近检查</dt><dd>{{ updateStatus.last_checked_at ? dayjs(updateStatus.last_checked_at).format("MM-DD HH:mm") : "—" }}</dd></div></dl>
      <n-alert v-if="updateStatus.error" type="error" :bordered="false">{{ translateBackendMessage(updateStatus.error) }}</n-alert>
      <div class="update-confirm">
        <n-input v-model:value="updateForm.confirmation" placeholder="输入 UPDATE 确认保存、备份、关服、更新并重启" />
        <n-button secondary :loading="action === 'check-update'" @click="checkUpdate">检查更新</n-button>
        <n-button type="warning" :disabled="updateForm.confirmation !== 'UPDATE'" :loading="action === 'apply-update'" @click="applyUpdate">执行更新</n-button>
      </div>
      <n-collapse class="learn-more"><n-collapse-item title="了解更新过程" name="update-help"><p>PST 只运行配置中的 steamcmd.exe，App ID 固定为 2394010。更新前会保存世界并创建备份，不接受自定义命令。</p></n-collapse-item></n-collapse>
    </section>

    <section v-else-if="activeTab === 'backup'" class="tool-panel">
      <header><div><h2>存档备份</h2><p>查看、下载或删除已经生成的备份。</p></div><n-button type="primary" :loading="action === 'backup'" @click="createBackup">立即备份</n-button></header>
      <div v-if="backups.length" class="backup-list">
        <article v-for="item in backups" :key="item.backup_id"><div><strong>{{ dayjs(item.save_time).format("YYYY-MM-DD HH:mm:ss") }} <n-tag size="tiny" :type="item.status === 'failed' ? 'error' : 'success'">{{ item.status === "failed" ? "失败" : "成功" }}</n-tag></strong><small>{{ item.path || item.error || "未生成备份文件" }} · {{ item.source === "manual" ? "手动" : item.source === "update" ? "更新前" : "自动" }} · {{ formatBytes(item.size) }}</small></div><n-space><n-button size="small" :disabled="item.status === 'failed' || !item.path" :loading="action === item.backup_id" @click="downloadBackup(item)">下载</n-button><n-button size="small" type="error" text @click="removeBackup(item)">删除</n-button></n-space></article>
      </div>
      <n-empty v-else description="还没有存档备份" class="empty" />
    </section>

    <section v-else-if="activeTab === 'rcon'" class="tool-panel">
      <header><div><h2>RCON 命令与定时任务</h2><p>已有 {{ commands.length }} 个命令模板、{{ tasks.length }} 个定时任务。</p></div><n-button type="primary" @click="showRcon = true">打开 RCON 控制台</n-button></header>
      <div class="rcon-summary"><article><strong>{{ commands.length }}</strong><span>命令模板</span></article><article><strong>{{ tasks.filter((item) => item.enabled).length }}</strong><span>启用的定时任务</span></article><article><strong>{{ tasks.filter((item) => item.last_status === 'failed').length }}</strong><span>最近失败</span></article></div>
      <div class="task-list"><article v-for="item in tasks.slice(0, 8)" :key="item.uuid"><div><strong>{{ item.name }}</strong><small>{{ item.cron }} · 执行 {{ item.run_count || 0 }} 次</small></div><n-tag size="small" :type="item.enabled ? item.last_status === 'failed' ? 'error' : 'success' : 'default'">{{ item.enabled ? item.last_status === "failed" ? "失败" : "已启用" : "已停用" }}</n-tag></article></div>
    </section>

    <section v-else-if="activeTab === 'logs'" class="tool-panel log-panel">
      <header><div><h2>PST 运行日志</h2><p>仅在当前标签打开时读取新日志。</p></div><n-select v-model:value="logLevel" clearable :options="[{label:'错误',value:'error'},{label:'警告',value:'warn'},{label:'信息',value:'info'},{label:'调试',value:'debug'}]" placeholder="全部级别" class="level-select" /></header>
      <div class="log-console"><article v-for="item in logs" :key="item.id" :class="item.level"><time>{{ dayjs(item.timestamp).format("HH:mm:ss") }}</time><b>{{ item.level.toUpperCase() }}</b><span>{{ item.message }}</span></article><n-empty v-if="!logs.length" description="暂无运行日志" /></div>
      <n-collapse class="learn-more"><n-collapse-item title="了解日志隐私" name="log-help"><p>密码、JWT 和 Authorization 等敏感内容会在返回页面前进行隐藏。</p></n-collapse-item></n-collapse>
    </section>

    <section v-else-if="activeTab === 'audit'" class="tool-panel">
      <header><div><h2>管理员操作记录</h2><p>查看管理操作的时间和结果。</p></div><n-button secondary @click="load">刷新</n-button></header>
      <div class="audit-list"><article v-for="item in audits" :key="item.id"><i :class="item.status" /><div><strong>{{ actionLabel(item.action) }}</strong><small>{{ item.detail }} · {{ dayjs(item.created_at).format("YYYY-MM-DD HH:mm:ss") }}</small></div><n-tag size="small" :type="item.status === 'success' ? 'success' : 'error'">{{ item.status === "success" ? "成功" : "失败" }}</n-tag></article></div>
      <n-collapse class="learn-more"><n-collapse-item title="了解记录内容" name="audit-help"><p>这里只保存操作名称、执行结果和时间，不保存密码或完整请求内容。</p></n-collapse-item></n-collapse>
    </section>
  </operations-shell>

  <config-manager v-model:show="showConfig" />
  <rcon-manager v-model:show="showRcon" />
</template>

<style scoped>
.ops-tabs { margin-bottom: 13px; }
.tab-error { margin-bottom: 12px; }
.retry-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.tab-skeleton { display: grid; gap: 14px; padding: 18px; border: 1px solid var(--ops-line); background: var(--ops-panel); }
.tool-panel { padding: 18px; border: 1px solid var(--ops-line); background: var(--ops-panel); }
.tool-panel > header { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; margin-bottom: 17px; }
h2 { margin: 5px 0; font-size: 23px; letter-spacing: -.025em; }
p { color: var(--ops-muted); font-size: 13px; line-height: 1.55; }
.learn-more { margin-top: 14px; border-top: 1px solid var(--ops-line); }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 14px; }
.preview { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding: 13px; background: var(--ops-accent-soft); }
.preview span, .preview small { color: var(--ops-muted); font-size: 11px; }
.preview b { flex: 1 0 100%; color: var(--ops-text); font-size: 12px; }
.preview b small { font-weight: 400; }
.preview .preview-error { flex: 1 0 100%; color: #c85d50; }
.preview strong { padding: 6px 8px; border: 1px solid rgba(23,141,121,.18); background: var(--ops-panel); font: 600 11px/1 ui-monospace, monospace; }
.schedule-panel footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: 18px; }
.update-facts, .rcon-summary { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 9px; margin: 0 0 14px; }
.update-facts div, .rcon-summary article { padding: 14px; background: var(--ops-accent-soft); }
dt, .rcon-summary span { color: var(--ops-muted); font-size: 11px; }
dd, .rcon-summary strong { display: block; margin: 5px 0 0; font: 700 20px/1 ui-monospace, monospace; }
.update-confirm { display: grid; grid-template-columns: minmax(260px, 1fr) auto auto; gap: 8px; margin-top: 15px; }
.backup-list article, .task-list article, .audit-list article { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 62px; border-top: 1px solid var(--ops-line); }
.backup-list strong, .backup-list small, .task-list strong, .task-list small, .audit-list strong, .audit-list small { display: block; }
.backup-list small, .task-list small, .audit-list small { margin-top: 3px; color: var(--ops-muted); font-size: 11px; }
.empty { padding: 70px 0; }
.level-select { width: 150px; }
.log-console { height: min(62dvh, 680px); padding: 10px 12px; background: #112019; color: #dcebe4; overflow: auto; overscroll-behavior: contain; font: 12px/1.55 ui-monospace, SFMono-Regular, Consolas, monospace; }
.log-console article { display: grid; grid-template-columns: 70px 58px minmax(0, 1fr); gap: 8px; padding: 3px 0; border-bottom: 1px solid rgba(220,235,228,.055); }
.log-console time { color: #7fa090; }
.log-console b { color: #77c8aa; }
.log-console .error b { color: #ff978d; }
.log-console .warn b { color: #e5bc72; }
.audit-list i { flex: 0 0 auto; width: 8px; height: 8px; border-radius: 50%; background: #d8796d; }
.audit-list i.success { background: var(--ops-accent); }
.audit-list div { flex: 1; min-width: 0; }
@media (max-width: 720px) {
  .tool-panel { padding: 14px; }
  .tool-panel > header { align-items: stretch; flex-direction: column; }
  .form-grid, .update-facts { grid-template-columns: 1fr; }
  .update-confirm { grid-template-columns: 1fr 1fr; }
  .update-confirm > :first-child { grid-column: 1 / -1; }
  .rcon-summary { grid-template-columns: repeat(3, 1fr); }
  .rcon-summary article { padding: 10px; }
  .rcon-summary strong { font-size: 17px; }
  .log-console { height: 56dvh; margin: 0 -14px -14px; }
  .log-console article { grid-template-columns: 56px 44px minmax(0, 1fr); font-size: 10px; }
}
</style>
