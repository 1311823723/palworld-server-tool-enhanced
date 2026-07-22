<script setup>
import { computed, nextTick, ref, watch } from "vue";
import dayjs from "dayjs";
import { useMessage } from "naive-ui";
import { useI18n } from "vue-i18n";
import ApiService from "@/service/api";
import userStore from "@/stores/model/user";
import DirectoryPicker from "@/components/DirectoryPicker.vue";

const props = defineProps({ show: Boolean });
const emit = defineEmits(["update:show"]);
const { t } = useI18n();
const message = useMessage();
const loading = ref(false);
const saving = ref(false);
const showDirectoryPicker = ref(false);
const newPassword = ref("");
const passwordConfirmation = ref("");
const saveTesting = ref(false);
const rconTesting = ref(false);
const saveStatus = ref({ status: "unconfigured", message: "" });
const rconStatus = ref({ status: "unconfigured", message: "" });
const sourcePaths = ref({ directory: "", agent: "" });

const statusType = (status) =>
  ({ normal: "success", error: "error", unconfigured: "warning" })[status] ||
  "default";
const statusLabel = (status) =>
  t(`configuration.connectionStatus.${status || "unconfigured"}`);
const restartFieldLabel = (field) =>
  ({
    "web.port": t("configuration.webPort"),
    "web.tls": "TLS",
    "web.cert_path": t("configuration.certPath"),
    "web.key_path": t("configuration.keyPath"),
    "web.public_url": t("configuration.publicUrl"),
    "task.sync_interval": t("configuration.playerSyncInterval"),
    "save.sync_interval": t("configuration.saveSyncInterval"),
    "save.backup_interval": t("configuration.backupInterval"),
  })[field] || field;

const emptySettings = () => ({
  web: {
    port: 8080,
    port_source: "",
    tls: false,
    cert_path: "",
    key_path: "",
    public_url: "",
  },
  task: {
    sync_interval: 60,
    player_logging: false,
    player_login_message: "",
    player_logout_message: "",
  },
  rcon: { address: "", password: "", use_base64: false, timeout: 5 },
  rest: { address: "", username: "admin", password: "", timeout: 5 },
  save: {
    source_mode: "directory",
    path: "",
    decode_path: "",
    sync_interval: 120,
    backup_interval: 14400,
    backup_keep_days: 7,
  },
  manage: { kick_non_whitelist: false },
  inventory_visibility: { mode: "admin", allow_public_summary: false },
  server_process: {
    enabled: false,
    executable_path: "",
    working_directory: "",
    arguments: [],
    watchdog_enabled: false,
    scheduled_restart_enabled: false,
    scheduled_restart_frequency: "daily",
    scheduled_restart_time: "04:00",
    scheduled_restart_interval_days: 2,
    scheduled_restart_start_date: dayjs().format("YYYY-MM-DD"),
    scheduled_restart_weekday: 1,
    scheduled_restart_day_of_month: 1,
    restart_delay_seconds: 10,
    graceful_shutdown_seconds: 30,
    graceful_shutdown_message: "服务器将在 30 秒后重启，请提前回到安全位置。",
    max_restart_attempts: 5,
    restart_attempt_window_seconds: 300,
  },
});
const settings = ref(emptySettings());
const scheduledRestartFieldsDisabled = computed(
  () =>
    !settings.value.server_process.enabled ||
    !settings.value.server_process.scheduled_restart_enabled,
);
const scheduledRestartFrequencyOptions = computed(() => [
  { label: t("configuration.scheduleDaily"), value: "daily" },
  {
    label: t("configuration.scheduleIntervalDays"),
    value: "interval_days",
  },
  { label: t("configuration.scheduleWeekly"), value: "weekly" },
  { label: t("configuration.scheduleMonthly"), value: "monthly" },
]);
const scheduledRestartWeekdayOptions = computed(() =>
  Array.from({ length: 7 }, (_, weekday) => ({
    value: weekday,
    label: t(`rconManager.weekday.${weekday}`),
  })),
);
const scheduledRestartMonthDayOptions = computed(() =>
  Array.from({ length: 31 }, (_, index) => ({
    value: index + 1,
    label: t("configuration.dayOfMonthOption", { day: index + 1 }),
  })),
);

const checkSaveSource = async () => {
  saveTesting.value = true;
  const { data, statusCode } = await new ApiService().testSaveConfig(
    settings.value.save,
  );
  saveTesting.value = false;
  if (statusCode.value !== 200) {
    saveStatus.value = {
      status: "error",
      message: data.value?.error || t("configuration.connectionTestFailed"),
    };
    return;
  }
  saveStatus.value = data.value;
};

const testRcon = async () => {
  rconTesting.value = true;
  const { data, statusCode } = await new ApiService().testRconConfig(
    settings.value.rcon,
  );
  rconTesting.value = false;
  if (statusCode.value !== 200) {
    rconStatus.value = {
      status: "error",
      message: data.value?.error || t("configuration.connectionTestFailed"),
    };
    return;
  }
  rconStatus.value = data.value;
};

const markRconDirty = () => {
  rconStatus.value = {
    status: "unconfigured",
    message: t("configuration.retestRequired"),
  };
};

const changeSourceMode = async (mode) => {
  const previousMode = settings.value.save.source_mode;
  sourcePaths.value[previousMode] = settings.value.save.path;
  settings.value.save.source_mode = mode;
  settings.value.save.path = sourcePaths.value[mode] || "";
  await nextTick();
  checkSaveSource();
};

const load = async () => {
  loading.value = true;
  const { data, statusCode } = await new ApiService().getConfig();
  loading.value = false;
  if (statusCode.value !== 200) {
    message.error(data.value?.error || t("configuration.loadFailed"));
    emit("update:show", false);
    return;
  }
  settings.value = { ...emptySettings(), ...data.value };
  settings.value.server_process = {
    ...emptySettings().server_process,
    ...(data.value.server_process || {}),
  };
  settings.value.inventory_visibility = {
    ...emptySettings().inventory_visibility,
    ...(data.value.inventory_visibility || {}),
  };
  sourcePaths.value[settings.value.save.source_mode] = settings.value.save.path;
  newPassword.value = "";
  passwordConfirmation.value = "";
  await Promise.all([checkSaveSource(), testRcon()]);
};

const save = async () => {
  if (newPassword.value !== passwordConfirmation.value) {
    message.error(t("configuration.passwordMismatch"));
    return;
  }
  saving.value = true;
  const { data, statusCode } = await new ApiService().updateConfig({
    settings: settings.value,
    new_password: newPassword.value,
  });
  saving.value = false;
  if (statusCode.value !== 200) {
    message.error(data.value?.error || t("configuration.saveFailed"));
    return;
  }
  if (data.value?.token) {
    localStorage.setItem("palworld_token", data.value.token);
    userStore().setIsLogin(true, data.value.token);
  }
  if (data.value?.restart_required) {
    const fields = (data.value.restart_fields || [])
      .map(restartFieldLabel)
      .join("、");
    message.warning(t("configuration.savedWithRestart", { fields }));
  } else {
    message.success(t("configuration.savedImmediately"));
  }
  emit("update:show", false);
};

const selectDirectory = (path) => {
  settings.value.save.path = path;
  sourcePaths.value.directory = path;
  showDirectoryPicker.value = false;
  checkSaveSource();
};

watch(
  () => props.show,
  (show) => {
    if (show) load();
  },
);

watch(
  () => settings.value.server_process.enabled,
  (enabled) => {
    if (!enabled) {
      settings.value.server_process.scheduled_restart_enabled = false;
    }
  },
);
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    class="config-card"
    style="width: min(94vw, 860px); max-height: calc(100vh - 64px)"
    content-style="overflow: hidden"
    :title="$t('configuration.title')"
    @update:show="emit('update:show', $event)"
  >
    <n-spin :show="loading">
      <n-alert type="warning" :bordered="false" class="mb-3">
        {{ $t("configuration.migrationWarning") }}
      </n-alert>
      <n-alert type="info" :bordered="false" class="mb-4">
        {{ $t("configuration.restartWarning") }}
      </n-alert>

      <n-scrollbar
        class="config-scroll"
        style="max-height: min(52vh, 560px); padding-right: 10px"
      >
        <n-collapse :default-expanded-names="['save', 'rcon', 'rest']">
          <n-collapse-item :title="$t('configuration.saveSection')" name="save">
            <template #header-extra>
              <n-spin v-if="saveTesting" size="small" />
              <n-tooltip v-else>
                <template #trigger>
                  <n-tag
                    size="small"
                    round
                    :type="statusType(saveStatus.status)"
                  >
                    {{ statusLabel(saveStatus.status) }}
                  </n-tag>
                </template>
                {{ saveStatus.message || $t("configuration.noStatusDetails") }}
              </n-tooltip>
            </template>
            <n-form label-placement="top">
              <n-form-item :label="$t('configuration.sourceMode')">
                <n-radio-group
                  :value="settings.save.source_mode"
                  @update:value="changeSourceMode"
                >
                  <n-space>
                    <n-radio value="directory">{{
                      $t("configuration.directoryMode")
                    }}</n-radio>
                    <n-radio value="agent">{{
                      $t("configuration.agentMode")
                    }}</n-radio>
                  </n-space>
                </n-radio-group>
              </n-form-item>
              <n-form-item
                :label="
                  settings.save.source_mode === 'agent'
                    ? $t('configuration.agentUrl')
                    : $t('configuration.saveDirectory')
                "
              >
                <n-input-group>
                  <n-input
                    v-model:value="settings.save.path"
                    @blur="checkSaveSource"
                    :placeholder="
                      settings.save.source_mode === 'agent'
                        ? 'http://game-server:8081/sync'
                        : $t('configuration.saveDirectoryPlaceholder')
                    "
                  />
                  <n-button
                    v-if="settings.save.source_mode === 'directory'"
                    @click="showDirectoryPicker = true"
                  >
                    {{ $t("configuration.browse") }}
                  </n-button>
                </n-input-group>
              </n-form-item>
              <div class="form-grid">
                <n-form-item :label="$t('configuration.decodePath')">
                  <n-input
                    v-model:value="settings.save.decode_path"
                    :placeholder="$t('configuration.autoDetect')"
                  />
                </n-form-item>
                <n-form-item :label="$t('configuration.saveSyncInterval')">
                  <n-input-number
                    v-model:value="settings.save.sync_interval"
                    :min="0"
                    class="full-width"
                  />
                </n-form-item>
                <n-form-item :label="$t('configuration.backupInterval')">
                  <n-input-number
                    v-model:value="settings.save.backup_interval"
                    :min="0"
                    class="full-width"
                  />
                </n-form-item>
                <n-form-item :label="$t('configuration.backupKeepDays')">
                  <n-input-number
                    v-model:value="settings.save.backup_keep_days"
                    :min="0"
                    class="full-width"
                  />
                </n-form-item>
              </div>
            </n-form>
          </n-collapse-item>

          <n-collapse-item title="RCON" name="rcon">
            <template #header-extra>
              <n-space size="small" align="center" @click.stop>
                <n-tooltip>
                  <template #trigger>
                    <n-tag
                      size="small"
                      round
                      :type="statusType(rconStatus.status)"
                    >
                      {{ statusLabel(rconStatus.status) }}
                    </n-tag>
                  </template>
                  {{
                    rconStatus.message || $t("configuration.noStatusDetails")
                  }}
                </n-tooltip>
                <n-button
                  size="tiny"
                  secondary
                  :loading="rconTesting"
                  @click.stop="testRcon"
                >
                  {{ $t("configuration.testConnection") }}
                </n-button>
              </n-space>
            </template>
            <n-form label-placement="top">
              <div class="form-grid">
                <n-form-item :label="$t('configuration.address')">
                  <n-input
                    v-model:value="settings.rcon.address"
                    placeholder="127.0.0.1:25575"
                    @update:value="markRconDirty"
                  />
                </n-form-item>
                <n-form-item :label="$t('configuration.password')">
                  <n-input
                    v-model:value="settings.rcon.password"
                    type="password"
                    show-password-on="click"
                    @update:value="markRconDirty"
                  />
                </n-form-item>
                <n-form-item :label="$t('configuration.timeout')">
                  <n-input-number
                    v-model:value="settings.rcon.timeout"
                    :min="0"
                    class="full-width"
                    @update:value="markRconDirty"
                  />
                </n-form-item>
                <n-form-item :label="$t('configuration.base64')">
                  <n-switch
                    v-model:value="settings.rcon.use_base64"
                    @update:value="markRconDirty"
                  />
                </n-form-item>
              </div>
            </n-form>
          </n-collapse-item>

          <n-collapse-item title="REST API" name="rest">
            <n-form label-placement="top">
              <div class="form-grid">
                <n-form-item :label="$t('configuration.address')">
                  <n-input
                    v-model:value="settings.rest.address"
                    placeholder="http://127.0.0.1:8212"
                  />
                </n-form-item>
                <n-form-item :label="$t('configuration.username')">
                  <n-input v-model:value="settings.rest.username" />
                </n-form-item>
                <n-form-item :label="$t('configuration.password')">
                  <n-input
                    v-model:value="settings.rest.password"
                    type="password"
                    show-password-on="click"
                  />
                </n-form-item>
                <n-form-item :label="$t('configuration.timeout')">
                  <n-input-number
                    v-model:value="settings.rest.timeout"
                    :min="0"
                    class="full-width"
                  />
                </n-form-item>
              </div>
            </n-form>
          </n-collapse-item>

          <n-collapse-item
            :title="$t('configuration.serverProcessSection')"
            name="server-process"
          >
            <n-alert type="warning" :bordered="false" class="mb-3">
              {{ $t("configuration.serverProcessSecurity") }}
            </n-alert>
            <n-form label-placement="top">
              <div class="form-grid">
                <n-form-item :label="$t('configuration.processEnabled')">
                  <n-switch v-model:value="settings.server_process.enabled" />
                </n-form-item>
                <n-form-item :label="$t('configuration.processWatchdog')">
                  <n-switch
                    v-model:value="settings.server_process.watchdog_enabled"
                  />
                </n-form-item>
              </div>
              <n-form-item :label="$t('configuration.executablePath')">
                <n-input
                  v-model:value="settings.server_process.executable_path"
                  placeholder="D:\\Program Files\\Steam\\steamapps\\common\\PalServer\\PalServer.exe"
                />
              </n-form-item>
              <n-form-item :label="$t('configuration.workingDirectory')">
                <n-input
                  v-model:value="settings.server_process.working_directory"
                  :placeholder="$t('configuration.workingDirectoryHint')"
                />
              </n-form-item>
              <n-form-item :label="$t('configuration.processArguments')">
                <n-dynamic-input
                  v-model:value="settings.server_process.arguments"
                  :placeholder="$t('configuration.processArgumentPlaceholder')"
                />
              </n-form-item>
              <div class="form-grid">
                <n-form-item :label="$t('configuration.restartDelaySeconds')">
                  <n-input-number
                    v-model:value="
                      settings.server_process.restart_delay_seconds
                    "
                    :min="0"
                    class="full-width"
                  />
                </n-form-item>
                <n-form-item
                  :label="$t('configuration.gracefulShutdownSeconds')"
                >
                  <n-input-number
                    v-model:value="
                      settings.server_process.graceful_shutdown_seconds
                    "
                    :min="0"
                    class="full-width"
                  />
                </n-form-item>
                <n-form-item :label="$t('configuration.maxRestartAttempts')">
                  <n-input-number
                    v-model:value="settings.server_process.max_restart_attempts"
                    :min="1"
                    class="full-width"
                  />
                </n-form-item>
                <n-form-item :label="$t('configuration.restartAttemptWindow')">
                  <n-input-number
                    v-model:value="
                      settings.server_process.restart_attempt_window_seconds
                    "
                    :min="1"
                    class="full-width"
                  />
                </n-form-item>
              </div>
              <n-form-item :label="$t('configuration.gracefulShutdownMessage')">
                <n-input
                  v-model:value="
                    settings.server_process.graceful_shutdown_message
                  "
                />
              </n-form-item>
              <n-divider title-placement="left">
                {{ $t("configuration.scheduledRestartSection") }}
              </n-divider>
              <n-alert type="info" :bordered="false" class="mb-3">
                {{ $t("configuration.scheduledRestartHint") }}
              </n-alert>
              <div class="form-grid">
                <n-form-item
                  :label="$t('configuration.scheduledRestartEnabled')"
                >
                  <n-switch
                    v-model:value="
                      settings.server_process.scheduled_restart_enabled
                    "
                    :disabled="!settings.server_process.enabled"
                  />
                </n-form-item>
                <n-form-item
                  :label="$t('configuration.scheduledRestartFrequency')"
                >
                  <n-select
                    v-model:value="
                      settings.server_process.scheduled_restart_frequency
                    "
                    :options="scheduledRestartFrequencyOptions"
                    :disabled="scheduledRestartFieldsDisabled"
                    class="full-width"
                  />
                </n-form-item>
                <n-form-item :label="$t('configuration.scheduledRestartTime')">
                  <n-time-picker
                    v-model:formatted-value="
                      settings.server_process.scheduled_restart_time
                    "
                    format="HH:mm"
                    value-format="HH:mm"
                    :seconds="false"
                    :clearable="false"
                    :disabled="scheduledRestartFieldsDisabled"
                    class="full-width"
                  />
                </n-form-item>
                <template
                  v-if="
                    settings.server_process.scheduled_restart_frequency ===
                    'interval_days'
                  "
                >
                  <n-form-item
                    :label="$t('configuration.scheduledRestartIntervalDays')"
                  >
                    <n-input-number
                      v-model:value="
                        settings.server_process.scheduled_restart_interval_days
                      "
                      :min="1"
                      :max="3650"
                      :disabled="scheduledRestartFieldsDisabled"
                      class="full-width"
                    />
                  </n-form-item>
                  <n-form-item
                    :label="$t('configuration.scheduledRestartStartDate')"
                  >
                    <n-date-picker
                      v-model:formatted-value="
                        settings.server_process.scheduled_restart_start_date
                      "
                      type="date"
                      format="yyyy-MM-dd"
                      value-format="yyyy-MM-dd"
                      :clearable="false"
                      :disabled="scheduledRestartFieldsDisabled"
                      class="full-width"
                    />
                  </n-form-item>
                </template>
                <n-form-item
                  v-else-if="
                    settings.server_process.scheduled_restart_frequency ===
                    'weekly'
                  "
                  :label="$t('configuration.scheduledRestartWeekday')"
                >
                  <n-select
                    v-model:value="
                      settings.server_process.scheduled_restart_weekday
                    "
                    :options="scheduledRestartWeekdayOptions"
                    :disabled="scheduledRestartFieldsDisabled"
                    class="full-width"
                  />
                </n-form-item>
                <n-form-item
                  v-else-if="
                    settings.server_process.scheduled_restart_frequency ===
                    'monthly'
                  "
                  :label="$t('configuration.scheduledRestartDayOfMonth')"
                  :feedback="$t('configuration.scheduledRestartMonthDayHint')"
                >
                  <n-select
                    v-model:value="
                      settings.server_process.scheduled_restart_day_of_month
                    "
                    :options="scheduledRestartMonthDayOptions"
                    :disabled="scheduledRestartFieldsDisabled"
                    class="full-width"
                  />
                </n-form-item>
              </div>
            </n-form>
          </n-collapse-item>

          <n-collapse-item
            :title="$t('configuration.tasksSection')"
            name="tasks"
          >
            <n-form label-placement="top">
              <div class="form-grid">
                <n-form-item :label="$t('configuration.playerSyncInterval')">
                  <n-input-number
                    v-model:value="settings.task.sync_interval"
                    :min="0"
                    class="full-width"
                  />
                </n-form-item>
                <n-form-item :label="$t('configuration.playerLogging')">
                  <n-switch v-model:value="settings.task.player_logging" />
                </n-form-item>
              </div>
              <n-form-item :label="$t('configuration.loginMessage')">
                <n-input
                  v-model:value="settings.task.player_login_message"
                  type="textarea"
                />
              </n-form-item>
              <n-form-item :label="$t('configuration.logoutMessage')">
                <n-input
                  v-model:value="settings.task.player_logout_message"
                  type="textarea"
                />
              </n-form-item>
              <n-checkbox v-model:checked="settings.manage.kick_non_whitelist">
                {{ $t("configuration.kickNonWhitelist") }}
              </n-checkbox>
            </n-form>
          </n-collapse-item>

          <n-collapse-item :title="$t('configuration.inventoryVisibilitySection')" name="inventory-visibility">
            <n-alert type="warning" :bordered="false" class="mb-3">{{ $t('configuration.inventoryVisibilityWarning') }}</n-alert>
            <n-form label-placement="top">
              <n-form-item :label="$t('configuration.inventoryVisibilityMode')">
                <n-radio-group v-model:value="settings.inventory_visibility.mode">
                  <n-space><n-radio value="admin">{{ $t('configuration.inventoryAdminOnly') }}</n-radio><n-radio value="public_summary">{{ $t('configuration.inventoryPublicSummary') }}</n-radio></n-space>
                </n-radio-group>
              </n-form-item>
              <n-checkbox v-model:checked="settings.inventory_visibility.allow_public_summary" :disabled="settings.inventory_visibility.mode !== 'public_summary'">{{ $t('configuration.inventoryPublicConfirmation') }}</n-checkbox>
            </n-form>
          </n-collapse-item>

          <n-collapse-item :title="$t('configuration.webSection')" name="web">
            <n-alert
              v-if="settings.web.port_source"
              type="warning"
              :bordered="false"
              class="mb-3"
            >
              {{
                $t("configuration.webPortOverrideWarning", {
                  port: settings.web.port,
                  source: $t(
                    `configuration.webPortOverrideSource.${settings.web.port_source}`,
                  ),
                })
              }}
            </n-alert>
            <n-form label-placement="top">
              <div class="form-grid">
                <n-form-item :label="$t('configuration.webPort')">
                  <n-input-number
                    v-model:value="settings.web.port"
                    :min="1"
                    :max="65535"
                    :disabled="Boolean(settings.web.port_source)"
                    class="full-width"
                  />
                </n-form-item>
                <n-form-item label="TLS">
                  <n-switch v-model:value="settings.web.tls" />
                </n-form-item>
                <n-form-item :label="$t('configuration.certPath')">
                  <n-input v-model:value="settings.web.cert_path" />
                </n-form-item>
                <n-form-item :label="$t('configuration.keyPath')">
                  <n-input v-model:value="settings.web.key_path" />
                </n-form-item>
              </div>
              <n-form-item :label="$t('configuration.publicUrl')">
                <n-input
                  v-model:value="settings.web.public_url"
                  placeholder="https://pst.example.com"
                />
              </n-form-item>
            </n-form>
          </n-collapse-item>

          <n-collapse-item
            :title="$t('configuration.securitySection')"
            name="security"
          >
            <n-alert type="info" :bordered="false" class="mb-3">
              {{ $t("configuration.panelPasswordNotice") }}
            </n-alert>
            <n-form label-placement="top">
              <div class="form-grid">
                <n-form-item :label="$t('configuration.newAdminPassword')">
                  <n-input
                    v-model:value="newPassword"
                    type="password"
                    show-password-on="click"
                    autocomplete="new-password"
                  />
                </n-form-item>
                <n-form-item :label="$t('configuration.confirmPassword')">
                  <n-input
                    v-model:value="passwordConfirmation"
                    type="password"
                    show-password-on="click"
                    autocomplete="new-password"
                  />
                </n-form-item>
              </div>
            </n-form>
          </n-collapse-item>
        </n-collapse>
      </n-scrollbar>
    </n-spin>

    <template #footer>
      <n-space justify="end">
        <n-button @click="emit('update:show', false)">{{
          $t("button.cancel")
        }}</n-button>
        <n-button type="primary" :loading="saving" @click="save">{{
          $t("button.save")
        }}</n-button>
      </n-space>
    </template>
  </n-modal>

  <directory-picker
    v-model:show="showDirectoryPicker"
    :initial-path="
      settings.save.source_mode === 'directory' ? settings.save.path : ''
    "
    @select="selectDirectory"
  />
</template>

<style scoped>
.config-scroll {
  min-height: 220px;
}
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}
.full-width {
  width: 100%;
}
@media (max-width: 700px) {
  .config-scroll {
    min-height: 160px;
  }
  .form-grid {
    grid-template-columns: 1fr;
    gap: 0;
  }
}
</style>
