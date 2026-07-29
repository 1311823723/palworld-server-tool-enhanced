<script setup>
import { computed, onMounted, reactive, ref } from "vue";
import { useDialog, useMessage } from "naive-ui";
import ApiService from "@/service/api";
import OperationsShell from "@/components/OperationsShell.vue";
import pageStore from "@/stores/model/page";

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
const categoryOptions = computed(() => Object.keys(categoryLabels).map((key) => ({ label: categoryLabels[key], value: key })));
const changed = (definition) => definition.secret ? Boolean(secretInputs[definition.key] || clearSecrets.value.includes(definition.key)) : JSON.stringify(values[definition.key]) !== JSON.stringify(current.value.values?.[definition.key]);
const visibleDefinitions = computed(() => schema.value.filter((definition) => {
  if (category.value && definition.category !== category.value) return false;
  if (riskyOnly.value && !definition.dangerous && !definition.performance_warning && !definition.secret) return false;
  if (modifiedOnly.value && !changed(definition)) return false;
  const q = search.value.trim().toLowerCase();
  return !q || definition.key.toLowerCase().includes(q) || definition.description_zh?.toLowerCase().includes(q) || definition.description_en?.toLowerCase().includes(q);
}));
const grouped = computed(() => visibleDefinitions.value.reduce((result, definition) => { (result[definition.category] ||= []).push(definition); return result; }, {}));
const modifiedCount = computed(() => schema.value.filter(changed).length);

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
  const [schemaResponse, currentResponse, backupResponse] = await Promise.all([api.getWorldSettingsSchema(), api.getWorldSettings(), api.getWorldSettingsBackups()]);
  loading.value = false;
  if (schemaResponse.statusCode.value !== 200 || currentResponse.statusCode.value !== 200) {
    message.error(currentResponse.data.value?.error || schemaResponse.data.value?.error || "世界设置读取失败"); return;
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
}

async function validateChanges() {
  if (!modifiedCount.value) { message.info("没有需要应用的改动"); return; }
  validating.value = true;
  const { data, statusCode } = await api.validateWorldSettings(requestBody());
  validating.value = false;
  if (statusCode.value !== 200) { message.error(data.value?.error || "校验失败"); return; }
  validation.value = data.value;
  showDiff.value = true;
}

async function applyChanges() {
  if (confirmText.value !== "应用") return;
  applying.value = true;
  const { data, statusCode } = await api.applyWorldSettings(requestBody());
  applying.value = false;
  if (statusCode.value !== 200) { message.error(data.value?.error || "应用失败；请查看进程状态和备份"); return; }
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

async function deleteBackup(id) {
  dialog.warning({ title: "删除设置备份", content: `确定删除 ${id}？`, positiveText: "删除", negativeText: "取消", onPositiveClick: async () => {
    const { data, statusCode } = await api.deleteWorldSettingsBackup(id);
    if (statusCode.value !== 200) message.error(data.value?.error || "删除失败"); else { message.success("备份已删除"); await load(); }
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
  const { data, statusCode } = await api.restoreWorldSettingsBackup(restoreTarget.value, {
    shutdown_seconds: 30,
    restart_delay_seconds: 10,
    message: "正在恢复服务器设置，将在 30 秒后重启。",
    confirmation: restoreConfirmText.value,
  });
  applying.value = false;
  if (statusCode.value !== 200) {
    message.error(data.value?.error || "恢复失败");
    return;
  }
  restoreVisible.value = false;
  message.success("备份已恢复");
  await load();
}

onMounted(load);
</script>

<template>
  <operations-shell title="世界设置" subtitle="按分类编辑 PalWorldSettings.ini。每次应用都会先备份、保存世界并通过平滑重启生效。">
    <n-alert type="warning" class="mb-4">仅支持由 PST 配置的 Windows 本地 PalServer。密码不会回显；留空表示保持不变，清空必须显式选择。关闭 REST API 会使 PST 的保存、关服和玩家功能不可用。</n-alert>
    <n-card size="small" class="mb-4">
      <n-descriptions :column="isMobile ? 1 : 3" size="small">
        <n-descriptions-item label="字段表版本">{{ schemaVersion }}</n-descriptions-item>
        <n-descriptions-item label="INI 修改时间">{{ current.modified_at || "—" }}</n-descriptions-item>
        <n-descriptions-item label="未知字段">{{ current.unknown_key_count || 0 }}（将原样保留）</n-descriptions-item>
        <n-descriptions-item label="配置文件" :span="isMobile ? 1 : 3"><n-ellipsis>{{ current.path || "未配置" }}</n-ellipsis></n-descriptions-item>
      </n-descriptions>
      <n-alert v-for="warning in current.parse_warnings || []" :key="warning" type="warning" class="mt-2">{{ warning }}</n-alert>
    </n-card>

    <n-card size="small">
      <template #header>设置字段 <n-tag type="info" size="small">已修改 {{ modifiedCount }}</n-tag></template>
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
                <n-switch v-else-if="definition.type === 'boolean'" v-model:value="values[definition.key]" />
                <n-input-number v-else-if="definition.type === 'integer' || definition.type === 'float'" v-model:value="values[definition.key]" :min="definition.minimum" :max="definition.maximum" :precision="definition.type === 'integer' ? 0 : undefined" clearable style="width:100%" />
                <n-select v-else-if="definition.type === 'enum'" v-model:value="values[definition.key]" :options="(definition.options || []).map(value => ({label:value,value}))" />
                <n-checkbox-group v-else-if="definition.type === 'platform_list'" v-model:value="values[definition.key]"><n-space><n-checkbox v-for="option in definition.options" :key="option" :value="option">{{ option }}</n-checkbox></n-space></n-checkbox-group>
                <n-dynamic-tags v-else-if="definition.type.endsWith('_list')" v-model:value="values[definition.key]" />
                <n-input v-else v-model:value="values[definition.key]" type="textarea" autosize />
                <small class="source">来源：{{ definition.source }} · 修改后需重启</small>
              </n-card>
            </div>
          </n-collapse-item>
        </n-collapse>
      </n-spin>
      <n-empty v-if="!loading && !visibleDefinitions.length" description="没有符合筛选条件的字段" />
      <n-space justify="end" class="mt-4"><n-button :disabled="!modifiedCount" :loading="validating" type="primary" @click="validateChanges">校验并查看差异</n-button></n-space>
    </n-card>

    <n-card title="设置备份" size="small" class="mt-4">
      <n-list bordered><n-list-item v-for="backup in backups" :key="backup.id"><n-thing :title="backup.id" :description="`${backup.created_at} · ${backup.size} bytes`" /><template #suffix><n-space><n-button size="small" :loading="applying" @click="restoreBackup(backup.id)">恢复</n-button><n-button size="small" type="error" secondary @click="deleteBackup(backup.id)">删除</n-button></n-space></template></n-list-item></n-list>
      <n-empty v-if="!backups.length" description="尚无 PST 世界设置备份" />
    </n-card>

    <n-modal v-model:show="showDiff" preset="card" title="确认世界设置变更" style="width:min(760px,94vw)">
      <n-alert v-for="warning in validation?.warnings || []" :key="warning" type="warning" class="mb-2">{{ warning }}</n-alert>
      <n-data-table :columns="[{title:'字段',key:'key'},{title:'原值',key:'before',render:r=>r.secret?'（敏感值已隐藏）':JSON.stringify(r.before)},{title:'新值',key:'after',render:r=>r.secret?'（敏感值已隐藏）':JSON.stringify(r.after)}]" :data="validation?.differences || []" :bordered="false" />
      <n-grid :cols="isMobile ? 1 : 3" :x-gap="12" class="mt-4"><n-gi><n-form-item label="关服倒计时"><n-input-number v-model:value="shutdownSeconds" :min="0" /></n-form-item></n-gi><n-gi><n-form-item label="重启等待"><n-input-number v-model:value="restartDelaySeconds" :min="0" /></n-form-item></n-gi><n-gi><n-form-item label="输入“应用”确认"><n-input v-model:value="confirmText" placeholder="应用" /></n-form-item></n-gi></n-grid>
      <n-form-item label="广播消息"><n-input v-model:value="restartMessage" /></n-form-item>
      <n-alert type="info">提交后将先保存世界并平滑关服，确认进程退出后才备份和写入；不会同时运行两个 PalServer。</n-alert>
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
.settings-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.settings-grid .modified{box-shadow:inset 3px 0 #18a058}.settings-grid .dangerous{border-color:rgba(208,48,80,.38)}.description{min-height:40px;margin:0 0 12px;opacity:.7}.source{display:block;margin-top:12px;opacity:.48}.mb-2{margin-bottom:8px}.mb-4{margin-bottom:16px}.mt-2{margin-top:8px}.mt-4{margin-top:16px}
@media(max-width:900px){.settings-grid{grid-template-columns:1fr}}@media(max-width:767px){.filters{display:grid!important}.filters>*{width:100%!important}}
</style>
