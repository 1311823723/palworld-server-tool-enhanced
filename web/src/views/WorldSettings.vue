<script setup>
import { computed, onMounted, reactive, ref } from "vue";
import { useDialog, useMessage } from "naive-ui";
import ApiService from "@/service/api";
import OperationsShell from "@/components/OperationsShell.vue";
import pageStore from "@/stores/model/page";
import { apiErrorText, translateBackendMessage } from "@/utils/apiError";

const api = new ApiService();
const message = useMessage();
const dialog = useDialog();
const loading = ref(false);
const validating = ref(false);
const applying = ref(false);
const schema = ref([]);
const schemaVersion = ref("");
const current = ref({ values: {}, secrets: {}, unknown_keys: [], parse_warnings: [] });
const values = reactive({});
const secretInputs = reactive({});
const clearSecrets = ref([]);
const backups = ref([]);
const search = ref("");
const category = ref("");
const riskyOnly = ref(false);
const modifiedOnly = ref(false);
const validation = ref(null);
const showDiff = ref(false);
const confirmText = ref("");
const restoreVisible = ref(false);
const restoreTarget = ref("");
const restoreConfirmText = ref("");
const shutdownSeconds = ref(30);
const restartDelaySeconds = ref(10);
const restartMessage = ref("服务器设置已修改，将在 30 秒后重启。");
const isMobile = computed(() => pageStore().getScreenWidth() < 768);

const categoryLabels = {
  basic: "基础", game_balance: "游戏平衡", server_management: "服务器管理", performance: "性能与容量", player: "玩家", pal: "帕鲁", base_building: "据点与建筑", drop_collection: "掉落与采集", death_penalty: "死亡惩罚", guild: "公会", features: "功能", pvp: "PvP", randomization: "随机化", rest_rcon: "REST / RCON", advanced: "高级", deprecated_reserved: "弃用与保留",
};
const commonSettingLabels = {
  ServerName: "服务器名称",
  ServerDescription: "服务器介绍",
  ServerPlayerMaxNum: "最大玩家数",
  Difficulty: "难度",
  ExpRate: "经验倍率",
  PalCaptureRate: "帕鲁捕获倍率",
  PalSpawnNumRate: "帕鲁出现数量倍率",
  PalEggDefaultHatchingTime: "巨大蛋孵化时间",
  WorkSpeedRate: "工作速度倍率",
  DeathPenalty: "死亡惩罚",
};
const commonSettingKeys = Object.keys(commonSettingLabels);
const categoryOptions = computed(() => Object.keys(categoryLabels).map((key) => ({ label: categoryLabels[key], value: key })));
const isReadonlySetting = (definition) => Boolean(definition.deprecated || definition.reserved);
const changed = (definition) => {
  // 弃用/保留字段不可修改，也不参与提交，避免后端校验拒绝。
  if (isReadonlySetting(definition)) return false;
  return definition.secret ? Boolean(secretInputs[definition.key] || clearSecrets.value.includes(definition.key)) : JSON.stringify(values[definition.key]) !== JSON.stringify(current.value.values?.[definition.key]);
};
const visibleDefinitions = computed(() => schema.value.filter((definition) => {
  if (category.value && definition.category !== category.value) return false;
  if (riskyOnly.value && !definition.dangerous && !definition.performance_warning && !definition.secret) return false;
  if (modifiedOnly.value && !changed(definition)) return false;
  const q = search.value.trim().toLowerCase();
  return !q || definition.key.toLowerCase().includes(q) || definition.description_zh?.toLowerCase().includes(q) || definition.description_en?.toLowerCase().includes(q);
}));
const grouped = computed(() => visibleDefinitions.value.reduce((result, definition) => { (result[definition.category] ||= []).push(definition); return result; }, {}));
const modifiedCount = computed(() => schema.value.filter(changed).length);
const commonDefinitions = computed(() => commonSettingKeys
  .map((key) => schema.value.find((definition) => definition.key === key))
  .filter(Boolean));

function requestBody() {
  const changes = {};
  const secrets = {};
  schema.value.forEach((definition) => {
    if (!changed(definition)) return;
    if (definition.secret) {
      if (secretInputs[definition.key]) secrets[definition.key] = secretInputs[definition.key];
    } else changes[definition.key] = values[definition.key];
  });
  return { changes, secrets, clear_secrets: clearSecrets.value, confirmation: confirmText.value, shutdown_seconds: shutdownSeconds.value, restart_delay_seconds: restartDelaySeconds.value, message: restartMessage.value };
}

async function load() {
  loading.value = true;
  try {
    const [schemaResponse, currentResponse, backupResponse] = await Promise.all([
      api.getWorldSettingsSchema(),
      api.getWorldSettings(),
      api.getWorldSettingsBackups(),
    ]);
    if (schemaResponse.statusCode.value !== 200 || currentResponse.statusCode.value !== 200) {
      const failed = currentResponse.statusCode.value !== 200 ? currentResponse : schemaResponse;
      message.error(apiErrorText(failed.data.value, "世界设置读取失败", failed.statusCode.value, failed.error?.value));
      return;
    }
    schema.value = schemaResponse.data.value?.settings || [];
    schemaVersion.value = schemaResponse.data.value?.schema_version || "";
    current.value = currentResponse.data.value || current.value;
    Object.keys(values).forEach((key) => delete values[key]);
    schema.value.forEach((definition) => {
      if (!definition.secret) values[definition.key] = current.value.values?.[definition.key] ?? definition.default;
      secretInputs[definition.key] = "";
    });
    clearSecrets.value = [];
    backups.value = backupResponse.statusCode.value === 200 ? backupResponse.data.value?.items || [] : [];
    validation.value = null;
    confirmText.value = "";
  } catch (error) {
    message.error(apiErrorText(null, "世界设置读取失败", 0, String(error)));
  } finally {
    loading.value = false;
  }
}

async function validateChanges() {
  if (!modifiedCount.value) { message.info("没有需要应用的改动"); return; }
  validating.value = true;
  const { data, statusCode, error } = await api.validateWorldSettings(requestBody());
  validating.value = false;
  if (statusCode.value !== 200) { message.error(apiErrorText(data.value, "校验失败", statusCode.value, error.value)); return; }
  validation.value = data.value;
  showDiff.value = true;
}

async function applyChanges() {
  if (confirmText.value !== "应用") return;
  applying.value = true;
  const { data, statusCode, error } = await api.applyWorldSettings(requestBody());
  applying.value = false;
  if (statusCode.value !== 200) { message.error(apiErrorText(data.value, "应用失败；请查看进程状态和备份", statusCode.value, error.value)); return; }
  message.success(`设置已应用，备份：${data.value?.backup_id}`);
  showDiff.value = false;
  await load();
}

function toggleClearSecret(key, enabled) {
  clearSecrets.value = enabled ? [...new Set([...clearSecrets.value, key])] : clearSecrets.value.filter((item) => item !== key);
  if (enabled) secretInputs[key] = "";
}

function resetField(definition) {
  if (definition.secret) { secretInputs[definition.key] = ""; toggleClearSecret(definition.key, false); }
  else values[definition.key] = current.value.values?.[definition.key] ?? definition.default;
}

function resetAll() {
  schema.value.forEach(resetField);
  validation.value = null;
  confirmText.value = "";
}

async function deleteBackup(id) {
  dialog.warning({ title: "删除设置备份", content: `确定删除 ${id}？`, positiveText: "删除", negativeText: "取消", onPositiveClick: async () => {
    const { data, statusCode, error } = await api.deleteWorldSettingsBackup(id);
    if (statusCode.value !== 200) message.error(apiErrorText(data.value, "删除失败", statusCode.value, error.value)); else { message.success("备份已删除"); await load(); }
  }});
}

function restoreBackup(id) {
  restoreTarget.value = id;
  restoreConfirmText.value = "";
  restoreVisible.value = true;
}

async function submitRestoreBackup() {
  if (restoreConfirmText.value !== "恢复" || applying.value) return;
  applying.value = true;
  const { data, statusCode, error } = await api.restoreWorldSettingsBackup(restoreTarget.value, {
    shutdown_seconds: 30,
    restart_delay_seconds: 10,
    message: "正在恢复服务器设置，将在 30 秒后重启。",
    confirmation: restoreConfirmText.value,
  });
  applying.value = false;
  if (statusCode.value !== 200) {
    message.error(apiErrorText(data.value, "恢复失败", statusCode.value, error.value));
    return;
  }
  restoreVisible.value = false;
  message.success("备份已恢复");
  await load();
}

onMounted(load);
</script>

<template>
  <operations-shell title="世界设置" subtitle="先调整常用选项，需要时再查看全部设置。">
    <section v-if="current.parse_warnings?.length" class="issue-panel mb-4">
      <h2>当前问题</h2>
      <n-alert v-for="warning in current.parse_warnings" :key="warning" type="warning">{{ translateBackendMessage(warning) }}</n-alert>
    </section>

    <n-card size="small" class="mb-4">
      <template #header>常用设置</template>
      <template #header-extra><n-tag v-if="modifiedCount" type="success" size="small">已修改 {{ modifiedCount }} 项</n-tag></template>
      <n-spin :show="loading">
        <div class="common-settings">
          <div v-for="definition in commonDefinitions" :key="definition.key" class="common-setting" :class="{ modified: changed(definition) }">
            <div class="common-label">
              <strong>{{ commonSettingLabels[definition.key] }}</strong>
              <code>{{ definition.key }}</code>
            </div>
            <div class="common-control">
              <n-switch v-if="definition.type === 'boolean'" v-model:value="values[definition.key]" />
              <n-input-number v-else-if="definition.type === 'integer' || definition.type === 'float'" v-model:value="values[definition.key]" :min="definition.minimum" :max="definition.maximum" :precision="definition.type === 'integer' ? 0 : undefined" clearable />
              <n-select v-else-if="definition.type === 'enum'" v-model:value="values[definition.key]" :options="(definition.options || []).map(value => ({ label: value, value }))" />
              <n-input v-else v-model:value="values[definition.key]" />
              <n-button v-if="changed(definition)" size="tiny" text @click="resetField(definition)">撤销</n-button>
            </div>
          </div>
        </div>
      </n-spin>
    </n-card>

    <n-card size="small">
      <template #header>全部设置</template>
      <template #header-extra><n-button size="small" :loading="loading" @click="load">重新读取</n-button></template>
      <n-space class="filters mb-4">
        <n-input v-model:value="search" clearable placeholder="搜索字段或说明" style="min-width:260px" />
        <n-select v-model:value="category" clearable placeholder="全部分类" :options="categoryOptions" style="width:180px" />
        <n-checkbox v-model:checked="modifiedOnly">只看修改</n-checkbox>
        <n-checkbox v-model:checked="riskyOnly">只看风险字段</n-checkbox>
      </n-space>
      <n-spin :show="loading">
        <n-collapse :default-expanded-names="['basic','server_management','performance']">
          <n-collapse-item v-for="(definitions, key) in grouped" :key="key" :name="key" :title="`${categoryLabels[key] || key} (${definitions.length})`">
            <div class="settings-grid">
              <n-card v-for="definition in definitions" :key="definition.key" size="small" :class="{ modified: changed(definition), dangerous: definition.dangerous }">
                <template #header><code>{{ definition.key }}</code></template>
                <template #header-extra><n-space size="small"><n-tag v-if="definition.secret" type="warning" size="small">敏感</n-tag><n-tag v-if="definition.dangerous" type="error" size="small">高风险</n-tag><n-tag v-else-if="definition.performance_warning" type="warning" size="small">性能</n-tag><n-button v-if="changed(definition)" text size="tiny" @click="resetField(definition)">撤销</n-button></n-space></template>
                <p class="description">{{ definition.description_zh || definition.description_en }}</p>
                <template v-if="definition.secret">
                  <n-tag size="small" :type="current.secrets?.[definition.key]?.is_set ? 'success' : 'default'">{{ current.secrets?.[definition.key]?.is_set ? "已设置" : "未设置" }}</n-tag>
                  <n-input v-model:value="secretInputs[definition.key]" type="password" show-password-on="click" placeholder="输入新值；留空保持不变" :disabled="clearSecrets.includes(definition.key)" class="mt-2" />
                  <n-checkbox :checked="clearSecrets.includes(definition.key)" @update:checked="value => toggleClearSecret(definition.key, value)" class="mt-2">显式清空该密码</n-checkbox>
                </template>
                <n-switch v-else-if="definition.type === 'boolean'" v-model:value="values[definition.key]" :disabled="isReadonlySetting(definition)" />
                <n-input-number v-else-if="definition.type === 'integer' || definition.type === 'float'" v-model:value="values[definition.key]" :min="definition.minimum" :max="definition.maximum" :precision="definition.type === 'integer' ? 0 : undefined" :disabled="isReadonlySetting(definition)" clearable style="width:100%" />
                <n-select v-else-if="definition.type === 'enum'" v-model:value="values[definition.key]" :options="(definition.options || []).map(value => ({label:value,value}))" :disabled="isReadonlySetting(definition)" />
                <n-checkbox-group v-else-if="definition.type === 'platform_list'" v-model:value="values[definition.key]" :disabled="isReadonlySetting(definition)"><n-space><n-checkbox v-for="option in definition.options" :key="option" :value="option">{{ option }}</n-checkbox></n-space></n-checkbox-group>
                <n-dynamic-tags v-else-if="definition.type.endsWith('_list')" v-model:value="values[definition.key]" :disabled="isReadonlySetting(definition)" />
                <n-input v-else v-model:value="values[definition.key]" type="textarea" autosize :disabled="isReadonlySetting(definition)" />
                <small class="source">来源：{{ definition.source }} · 修改后需重启</small>
              </n-card>
            </div>
          </n-collapse-item>
        </n-collapse>
      </n-spin>
      <n-empty v-if="!loading && !visibleDefinitions.length" description="没有符合筛选条件的字段" />
      <n-collapse class="settings-help mt-4">
        <n-collapse-item title="了解设置文件与安全规则" name="settings-help">
          <n-descriptions :column="isMobile ? 1 : 3" size="small">
            <n-descriptions-item label="字段表版本">{{ schemaVersion }}</n-descriptions-item>
            <n-descriptions-item label="INI 修改时间">{{ current.modified_at || "—" }}</n-descriptions-item>
            <n-descriptions-item label="未知字段">{{ current.unknown_key_count || 0 }}（原样保留）</n-descriptions-item>
            <n-descriptions-item label="配置文件" :span="isMobile ? 1 : 3"><n-ellipsis>{{ current.path || "未配置" }}</n-ellipsis></n-descriptions-item>
          </n-descriptions>
          <p class="help-copy">密码不会回显；留空表示保持不变。关闭 REST API 后，保存世界、平滑关服和玩家查询等功能将不可用。</p>
        </n-collapse-item>
      </n-collapse>
    </n-card>

    <n-card title="设置备份" size="small" class="mt-4">
      <n-list bordered><n-list-item v-for="backup in backups" :key="backup.id"><n-thing :title="backup.id" :description="`${backup.created_at} · ${backup.size} bytes`" /><template #suffix><n-space><n-button size="small" :loading="applying" @click="restoreBackup(backup.id)">恢复</n-button><n-button size="small" type="error" secondary @click="deleteBackup(backup.id)">删除</n-button></n-space></template></n-list-item></n-list>
      <n-empty v-if="!backups.length" description="尚无 PST 世界设置备份" />
    </n-card>

    <div v-if="modifiedCount" class="settings-action-bar">
      <strong>已修改 {{ modifiedCount }} 项</strong>
      <span>尚未写入服务器</span>
      <n-button secondary :disabled="validating || applying" @click="resetAll">撤销全部</n-button>
      <n-button type="primary" :loading="validating" :disabled="applying" @click="validateChanges">校验并查看差异</n-button>
    </div>

    <n-modal v-model:show="showDiff" preset="card" title="确认世界设置变更" style="width:min(760px,94vw)">
      <n-alert v-for="warning in validation?.warnings || []" :key="warning" type="warning" class="mb-2">{{ warning }}</n-alert>
      <n-data-table :columns="[{title:'字段',key:'key'},{title:'原值',key:'before',render:r=>r.secret?'（敏感值已隐藏）':JSON.stringify(r.before)},{title:'新值',key:'after',render:r=>r.secret?'（敏感值已隐藏）':JSON.stringify(r.after)}]" :data="validation?.differences || []" :bordered="false" />
      <n-grid :cols="isMobile ? 1 : 3" :x-gap="12" class="mt-4"><n-gi><n-form-item label="关服倒计时"><n-input-number v-model:value="shutdownSeconds" :min="0" /></n-form-item></n-gi><n-gi><n-form-item label="重启等待"><n-input-number v-model:value="restartDelaySeconds" :min="0" /></n-form-item></n-gi><n-gi><n-form-item label="输入“应用”确认"><n-input v-model:value="confirmText" placeholder="应用" /></n-form-item></n-gi></n-grid>
      <n-form-item label="广播消息"><n-input v-model:value="restartMessage" /></n-form-item>
      <n-collapse><n-collapse-item title="了解应用过程" name="apply-help"><p class="help-copy">提交后会先保存世界并平滑关服，确认进程退出后再备份和写入设置。</p></n-collapse-item></n-collapse>
      <template #footer><n-space justify="end"><n-button @click="showDiff=false">取消</n-button><n-button type="error" :disabled="confirmText !== '应用'" :loading="applying" @click="applyChanges">应用并重启</n-button></n-space></template>
    </n-modal>

    <n-modal v-model:show="restoreVisible" preset="card" title="恢复设置备份" style="width:min(560px,94vw)" :mask-closable="false">
      <n-alert type="error" class="mb-4">
        将恢复 {{ restoreTarget }}。PST 会保存世界、等待 PalServer 完全退出、写入备份，然后重新启动服务器。
      </n-alert>
      <n-form-item label="输入“恢复”确认">
        <n-input v-model:value="restoreConfirmText" placeholder="恢复" autocomplete="off" @keyup.enter="submitRestoreBackup" />
      </n-form-item>
      <template #footer>
        <n-space justify="end">
          <n-button :disabled="applying" @click="restoreVisible=false">取消</n-button>
          <n-button type="error" :disabled="restoreConfirmText !== '恢复'" :loading="applying" @click="submitRestoreBackup">恢复并重启</n-button>
        </n-space>
      </template>
    </n-modal>
  </operations-shell>
</template>

<style scoped>
.issue-panel{display:grid;gap:8px}.issue-panel h2{margin:0;font-size:16px}.common-settings{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1px;border:1px solid var(--ops-line);background:var(--ops-line)}.common-setting{display:grid;grid-template-columns:minmax(140px,.8fr) minmax(180px,1.2fr);align-items:center;gap:16px;min-height:72px;padding:12px 14px;background:var(--ops-panel)}.common-setting.modified{box-shadow:inset 3px 0 #18a058}.common-label strong,.common-label code{display:block}.common-label strong{font-size:14px}.common-label code{margin-top:4px;color:var(--ops-muted);font-size:10px}.common-control{display:flex;align-items:center;gap:8px}.common-control>:first-child{flex:1}.settings-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.settings-grid .modified{box-shadow:inset 3px 0 #18a058}.settings-grid .dangerous{border-color:rgba(208,48,80,.38)}.description{min-height:40px;margin:0 0 12px;opacity:.7}.source{display:block;margin-top:12px;opacity:.48}.settings-help{border-top:1px solid var(--ops-line)}.help-copy{margin:12px 0 0;color:var(--ops-muted);font-size:13px;line-height:1.65}.settings-action-bar{position:fixed;z-index:35;right:clamp(20px,4vw,52px);bottom:16px;left:calc(228px + clamp(20px,4vw,52px));display:flex;align-items:center;gap:10px;max-width:1320px;margin:auto;padding:11px 14px;border:1px solid rgba(23,141,121,.28);border-radius:12px;background:rgba(255,255,255,.96);box-shadow:0 14px 38px rgba(31,57,48,.18);backdrop-filter:blur(16px)}.settings-action-bar strong{font-size:14px}.settings-action-bar span{flex:1;color:var(--ops-muted);font-size:12px}.mb-2{margin-bottom:8px}.mb-4{margin-bottom:16px}.mt-2{margin-top:8px}.mt-4{margin-top:16px}
@media(max-width:1000px){.common-settings,.settings-grid{grid-template-columns:1fr}}@media(max-width:767px){.filters{display:grid!important}.filters>*{width:100%!important}.common-setting{grid-template-columns:1fr;gap:9px}.settings-action-bar{right:12px;bottom:66px;left:12px;display:grid;grid-template-columns:1fr 1fr}.settings-action-bar strong,.settings-action-bar span{grid-column:1/-1}.settings-action-bar span{margin-top:-7px}}
</style>
