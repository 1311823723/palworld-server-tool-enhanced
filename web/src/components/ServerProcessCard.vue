<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from "vue";
import dayjs from "dayjs";
import { useMessage } from "naive-ui";
import { useI18n } from "vue-i18n";
import ApiService from "@/service/api";

defineProps({ isAdmin: { type: Boolean, default: true } });

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
  message: "Server restart in 30 seconds",
  confirmation: "",
});
const stopForm = reactive({
  shutdown_seconds: 30,
  message: "Server shutting down in 30 seconds",
  keep_stopped: true,
  confirmation: "",
});
let pollTimer;
let disposed = false;

const busy = computed(() =>
  ["starting", "stopping", "restart_waiting", "restarting"].includes(
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
const uptime = computed(() => {
  const seconds = Number(status.value.uptime_seconds || 0);
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return days > 0 ? `${days}d ${hours}h ${minutes}m` : `${hours}h ${minutes}m`;
});
const formatTime = (value) =>
  value ? dayjs(value).format("YYYY-MM-DD HH:mm:ss") : "—";

const showError = (data, fallback) =>
  message.error(data?.error || fallback || t("serverProcess.actionFailed"));

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
  const { data, statusCode } = await api.getServerProcess();
  loading.value = false;
  if (statusCode.value === 200) {
    status.value = data.value;
    if (notify) message.success(t("serverProcess.refreshed"));
  } else {
    showError(data.value, t("serverProcess.loadFailed"));
  }
  schedulePoll();
};

const runAction = async (name, request, successKey) => {
  if (action.value) return false;
  action.value = name;
  const { data, statusCode } = await request();
  action.value = "";
  if (statusCode.value < 200 || statusCode.value >= 300) {
    showError(data.value);
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
    restartForm.confirmation = "";
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
    stopForm.confirmation = "";
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

    <n-descriptions
      responsive="screen"
      :column="3"
      label-placement="top"
      bordered
    >
      <n-descriptions-item label="PID">{{
        status.pid || "—"
      }}</n-descriptions-item>
      <n-descriptions-item :label="$t('serverProcess.uptime')">{{
        uptime
      }}</n-descriptions-item>
      <n-descriptions-item :label="$t('serverProcess.watchdog')">
        <n-tag
          size="small"
          :type="status.watchdog_enabled ? 'success' : 'default'"
        >
          {{
            status.watchdog_enabled
              ? $t("serverProcess.enabled")
              : $t("serverProcess.disabled")
          }}
        </n-tag>
      </n-descriptions-item>
      <n-descriptions-item :label="$t('serverProcess.lastExit')">{{
        formatTime(status.last_exit_at)
      }}</n-descriptions-item>
      <n-descriptions-item :label="$t('serverProcess.exitCode')">{{
        status.last_exit_code ?? "—"
      }}</n-descriptions-item>
      <n-descriptions-item :label="$t('serverProcess.restartCount')"
        >{{ status.restart_count || 0 }} /
        {{ status.recent_crash_count || 0 }}</n-descriptions-item
      >
    </n-descriptions>
    <n-text v-if="status.last_error" type="error" class="error-text">
      {{ $t("serverProcess.lastError") }}: {{ status.last_error }}
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
    </n-flex>
  </n-card>

  <n-modal
    v-model:show="restartVisible"
    preset="dialog"
    :title="$t('serverProcess.restartTitle')"
    :positive-text="$t('serverProcess.restart')"
    :negative-text="$t('button.cancel')"
    :positive-button-props="{
      disabled: restartForm.confirmation !== 'RESTART',
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
      <n-form-item :label="$t('serverProcess.confirmRestart')"
        ><n-input
          v-model:value="restartForm.confirmation"
          placeholder="RESTART"
      /></n-form-item>
    </n-form>
  </n-modal>

  <n-modal
    v-model:show="stopVisible"
    preset="dialog"
    :title="$t('serverProcess.stopTitle')"
    :positive-text="$t('serverProcess.stop')"
    :negative-text="$t('button.cancel')"
    :positive-button-props="{
      disabled: stopForm.confirmation !== 'SHUTDOWN',
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
      <n-form-item :label="$t('serverProcess.confirmStop')"
        ><n-input v-model:value="stopForm.confirmation" placeholder="SHUTDOWN"
      /></n-form-item>
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
</style>
