<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from "vue";
import dayjs from "dayjs";
import { useMessage } from "naive-ui";
import { useI18n } from "vue-i18n";
import ApiService from "@/service/api";
import { apiErrorText, translateBackendMessage } from "@/utils/apiError";

const props = defineProps({
  isAdmin: { type: Boolean, default: true },
  compact: { type: Boolean, default: false },
});

const { t } = useI18n();
const message = useMessage();
const api = new ApiService();
const status = ref({ state: "stopped", running: false });
const loading = ref(false);
const action = ref("");
const restartVisible = ref(false);
const stopVisible = ref(false);
const restartForm = reactive({
  shutdown_seconds: 30,
  restart_delay_seconds: 10,
  message: "服务器将在 30 秒后重启，请提前回到安全位置。",
});
const stopForm = reactive({
  shutdown_seconds: 30,
  message: "服务器将在 30 秒后关闭，请提前回到安全位置。",
  keep_stopped: true,
});
let pollTimer;
let disposed = false;

const busy = computed(() =>
  ["starting", "stopping", "restart_waiting", "restarting", "updating"].includes(
    status.value.state,
  ),
);
const stateType = computed(
  () =>
    ({
      running: "success",
      stopped: "default",
      starting: "info",
      stopping: "warning",
      restart_waiting: "warning",
      restarting: "warning",
      crash_loop_stopped: "error",
      error: "error",
    })[status.value.state] || "default",
);
const stateLabel = computed(() =>
  t(`serverProcess.states.${status.value.state || "stopped"}`),
);
const scheduledRestartLabel = computed(() => {
  const value = status.value;
  const time = value.scheduled_restart_time || "04:00";
  switch (value.scheduled_restart_frequency || "daily") {
    case "interval_days":
      return t("serverProcess.intervalDaysAt", {
        days: value.scheduled_restart_interval_days || 1,
        time,
      });
    case "weekly":
      return t("serverProcess.weeklyAt", {
        weekday: t(
          `rconManager.weekday.${value.scheduled_restart_weekday ?? 1}`,
        ),
        time,
      });
    case "monthly":
      return t("serverProcess.monthlyAt", {
        day: value.scheduled_restart_day_of_month || 1,
        time,
      });
    case "cron":
      return `Cron：${value.scheduled_restart_cron || "—"}`;
    default:
      return t("serverProcess.dailyAt", { time });
  }
});
const uptime = computed(() => {
  const seconds = Number(status.value.uptime_seconds || 0);
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return days > 0 ? `${days} 天 ${hours} 小时` : `${hours} 小时 ${minutes} 分钟`;
});
const formatTime = (value) =>
  value ? dayjs(value).format("YYYY-MM-DD HH:mm:ss") : "—";
const formatServerTime = (value) =>
  value ? String(value).replace("T", " ").slice(0, 19) : "—";

const showError = (data, fallback, statusCode, requestError) =>
  message.error(apiErrorText(
    data,
    fallback || t("serverProcess.actionFailed"),
    statusCode,
    requestError,
  ));

const schedulePoll = () => {
  if (disposed) return;
  clearTimeout(pollTimer);
  pollTimer = setTimeout(
    loadStatus,
    busy.value || status.value.restarting ? 2000 : 10000,
  );
};

const loadStatus = async (notify = false) => {
  loading.value = true;
  const { data, statusCode, error } = await api.getServerProcess();
  loading.value = false;
  if (statusCode.value === 200) {
    status.value = data.value;
    if (notify) message.success(t("serverProcess.refreshed"));
  } else {
    showError(data.value, t("serverProcess.loadFailed"), statusCode.value, error.value);
  }
  schedulePoll();
};

const runAction = async (name, request, successKey) => {
  if (action.value) return false;
  action.value = name;
  const { data, statusCode, error } = await request();
  action.value = "";
  if (statusCode.value < 200 || statusCode.value >= 300) {
    showError(data.value, undefined, statusCode.value, error.value);
    return false;
  }
  if (data.value?.state) status.value = data.value;
  message.success(t(successKey));
  schedulePoll();
  return true;
};

const saveWorld = () =>
  runAction("save", () => api.saveServer(), "serverProcess.saveRequested");
const startServer = () =>
  runAction("start", () => api.startServer(), "serverProcess.startRequested");
const toggleWatchdog = () =>
  runAction(
    "watchdog",
    () => api.setServerWatchdog({ enabled: !status.value.watchdog_enabled }),
    status.value.watchdog_enabled
      ? "serverProcess.watchdogDisabled"
      : "serverProcess.watchdogEnabled",
  );

const submitRestart = async () => {
  const succeeded = await runAction(
    "restart",
    () =>
      api.restartServer({
        shutdown_seconds: restartForm.shutdown_seconds,
        restart_delay_seconds: restartForm.restart_delay_seconds,
        message: restartForm.message,
      }),
    "serverProcess.restartRequested",
  );
  if (succeeded) {
    restartVisible.value = false;
  }
};

const submitStop = async () => {
  const succeeded = await runAction(
    "stop",
    () =>
      api.stopServer({
        shutdown_seconds: stopForm.shutdown_seconds,
        message: stopForm.message,
        keep_stopped: stopForm.keep_stopped,
      }),
    "serverProcess.stopRequested",
  );
  if (succeeded) {
    stopVisible.value = false;
  }
};

onMounted(loadStatus);
onBeforeUnmount(() => {
  disposed = true;
  clearTimeout(pollTimer);
});
</script>

<template>
  <n-card :title="$t('serverProcess.title')">
    <template #header-extra>
      <n-space align="center">
        <n-tag :type="stateType" round>{{ stateLabel }}</n-tag>
        <n-button
          size="small"
          quaternary
          :loading="loading"
          @click="loadStatus(true)"
        >
          {{ $t("serverProcess.refresh") }}
        </n-button>
      </n-space>
    </template>

    <div v-if="status.crash_loop_detected || status.external_process || status.last_error || status.last_scheduled_restart_error" class="issue-heading">当前问题</div>
    <n-alert
      v-if="status.crash_loop_detected"
      type="error"
      :bordered="false"
      class="mb-4"
    >
      {{ $t("serverProcess.crashLoopWarning") }}
    </n-alert>
    <n-alert
      v-else-if="status.external_process"
      type="info"
      :bordered="false"
      class="mb-4"
    >
      {{ $t("serverProcess.externalWarning") }}
    </n-alert>

    <n-text v-if="status.last_error" type="error" class="error-text">
      {{ $t("serverProcess.lastError") }}：{{ translateBackendMessage(status.last_error) }}
    </n-text>
    <n-text
      v-if="status.last_scheduled_restart_error"
      type="warning"
      class="error-text"
    >
      {{ $t("serverProcess.lastScheduledRestartError") }}:
      {{ translateBackendMessage(status.last_scheduled_restart_error) }}
    </n-text>

    <n-flex v-if="isAdmin" class="mt-4" :size="10">
      <n-button
        secondary
        :loading="action === 'save'"
        :disabled="Boolean(action) || !status.running"
        @click="saveWorld"
      >
        {{ $t("serverProcess.saveWorld") }}
      </n-button>
      <n-button
        type="primary"
        secondary
        :loading="action === 'start'"
        :disabled="Boolean(action) || status.running || busy"
        @click="startServer"
      >
        {{ $t("serverProcess.start") }}
      </n-button>
      <n-button
        type="warning"
        secondary
        :disabled="Boolean(action) || !status.running || busy"
        @click="restartVisible = true"
      >
        {{ $t("serverProcess.restart") }}
      </n-button>
      <n-button
        type="error"
        secondary
        :disabled="Boolean(action) || !status.running || busy"
        @click="stopVisible = true"
      >
        {{ $t("serverProcess.stop") }}
      </n-button>
      <n-button
        secondary
        :loading="action === 'watchdog'"
        :disabled="Boolean(action)"
        @click="toggleWatchdog"
      >
        {{
          status.watchdog_enabled
            ? $t("serverProcess.disableWatchdog")
            : $t("serverProcess.enableWatchdog")
        }}
      </n-button>
      <slot name="actions" />
    </n-flex>

    <n-collapse
      class="process-details"
      :default-expanded-names="props.compact ? [] : ['details']"
    >
      <n-collapse-item title="详细状态" name="details">
        <n-descriptions responsive="screen" :column="3" label-placement="top" bordered>
          <n-descriptions-item label="PID">{{ status.pid || "—" }}</n-descriptions-item>
          <n-descriptions-item :label="$t('serverProcess.uptime')">{{ uptime }}</n-descriptions-item>
          <n-descriptions-item :label="$t('serverProcess.watchdog')">
            <n-tag size="small" :type="status.watchdog_enabled ? 'success' : 'default'">
              {{ status.watchdog_enabled ? $t("serverProcess.enabled") : $t("serverProcess.disabled") }}
            </n-tag>
          </n-descriptions-item>
          <n-descriptions-item :label="$t('serverProcess.lastExit')">{{ formatTime(status.last_exit_at) }}</n-descriptions-item>
          <n-descriptions-item :label="$t('serverProcess.exitCode')">{{ status.last_exit_code ?? "—" }}</n-descriptions-item>
          <n-descriptions-item :label="$t('serverProcess.restartCount')">{{ status.restart_count || 0 }} / {{ status.recent_crash_count || 0 }}</n-descriptions-item>
          <n-descriptions-item :label="$t('serverProcess.scheduledRestart')">
            <n-tag size="small" :type="status.scheduled_restart_enabled ? 'success' : 'default'">
              {{ status.scheduled_restart_enabled ? scheduledRestartLabel : $t("serverProcess.disabled") }}
            </n-tag>
          </n-descriptions-item>
          <n-descriptions-item :label="$t('serverProcess.nextScheduledRestart')">{{ formatServerTime(status.next_scheduled_restart_at) }}</n-descriptions-item>
          <n-descriptions-item :label="$t('serverProcess.scheduleTimezone')">{{ status.scheduled_restart_timezone || "—" }}</n-descriptions-item>
          <n-descriptions-item :label="$t('serverProcess.lastScheduledRestart')">{{ formatServerTime(status.last_scheduled_restart_at) }}</n-descriptions-item>
        </n-descriptions>
      </n-collapse-item>
    </n-collapse>
  </n-card>

  <n-modal
    v-model:show="restartVisible"
    preset="dialog"
    :title="$t('serverProcess.restartTitle')"
    :positive-text="$t('serverProcess.restart')"
    :negative-text="$t('button.cancel')"
    :mask-closable="false"
    :positive-button-props="{
      disabled: Boolean(action && action !== 'restart'),
      loading: action === 'restart',
    }"
    @positive-click="submitRestart"
  >
    <n-form label-placement="top" class="mt-3">
      <n-form-item :label="$t('serverProcess.shutdownSeconds')"
        ><n-input-number
          v-model:value="restartForm.shutdown_seconds"
          :min="0"
          :max="3600"
          class="full-width"
      /></n-form-item>
      <n-form-item :label="$t('serverProcess.restartDelay')"
        ><n-input-number
          v-model:value="restartForm.restart_delay_seconds"
          :min="0"
          :max="3600"
          class="full-width"
      /></n-form-item>
      <n-form-item :label="$t('serverProcess.message')"
        ><n-input v-model:value="restartForm.message" type="textarea"
      /></n-form-item>
      <n-alert type="warning" :bordered="false">
        确认后将先广播并保存世界，等 PalServer 完全退出后再启动。
      </n-alert>
    </n-form>
  </n-modal>

  <n-modal
    v-model:show="stopVisible"
    preset="dialog"
    :title="$t('serverProcess.stopTitle')"
    :positive-text="$t('serverProcess.stop')"
    :negative-text="$t('button.cancel')"
    :mask-closable="false"
    :positive-button-props="{
      disabled: Boolean(action && action !== 'stop'),
      loading: action === 'stop',
      type: 'error',
    }"
    @positive-click="submitStop"
  >
    <n-form label-placement="top" class="mt-3">
      <n-form-item :label="$t('serverProcess.shutdownSeconds')"
        ><n-input-number
          v-model:value="stopForm.shutdown_seconds"
          :min="0"
          :max="3600"
          class="full-width"
      /></n-form-item>
      <n-form-item :label="$t('serverProcess.message')"
        ><n-input v-model:value="stopForm.message" type="textarea"
      /></n-form-item>
      <n-form-item :label="$t('serverProcess.keepStopped')"
        ><n-switch v-model:value="stopForm.keep_stopped"
      /></n-form-item>
      <n-alert type="error" :bordered="false">
        确认后将保存世界并平滑关服。开启“关闭后保持停止”时，守护程序不会自动拉起服务器。
      </n-alert>
    </n-form>
  </n-modal>
</template>

<style scoped>
.full-width {
  width: 100%;
}
.error-text {
  display: block;
  margin-top: 12px;
  overflow-wrap: anywhere;
}
.issue-heading {
  margin-bottom: 8px;
  color: var(--ops-muted, #6c7b74);
  font-size: 12px;
  font-weight: 650;
}
.process-details {
  margin-top: 14px;
}
</style>
