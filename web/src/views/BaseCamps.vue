<script setup>
import { computed, onMounted, ref } from "vue";
import { useDialog, useMessage } from "naive-ui";
import ApiService from "@/service/api";
import OperationsShell from "@/components/OperationsShell.vue";

const api = new ApiService();
const message = useMessage();
const dialog = useDialog();
const isAdmin = computed(() => Boolean(localStorage.getItem("palworld_token")));
const loading = ref(false);
const saving = ref(false);
const bases = ref([]);
const aliases = ref([]);
const metadata = ref({ warnings: [] });
const editorOpen = ref(false);
const editingBase = ref(null);
const aliasName = ref("");

const inactiveAliases = computed(() => aliases.value.filter((item) => !item.active));
const shortID = (value) => value ? `${value.slice(0, 6)}…${value.slice(-6)}` : "—";
const coordinate = (base) => [base.location?.x, base.location?.y, base.location?.z]
  .map((value) => Number(value || 0).toFixed(0)).join(", ");
const attentionCount = (base) => Number(base.hungry_pal_count || 0)
  + Number(base.low_sanity_pal_count || 0)
  + Number(base.sick_pal_count || 0)
  + Number(base.down_pal_count || 0);
const trimmedAlias = computed(() => aliasName.value.trim());
const duplicateAlias = computed(() => bases.value.some((base) =>
  base.base_id !== editingBase.value?.base_id
  && String(base.display_name || "").trim().toLocaleLowerCase("zh-CN") === trimmedAlias.value.toLocaleLowerCase("zh-CN")));
const aliasError = computed(() => {
  if (!trimmedAlias.value) return "请输入据点名称";
  if ([...trimmedAlias.value].length > 40) return "名称不能超过 40 个字符";
  if (/[\r\n\u0000-\u001f\u007f]/u.test(trimmedAlias.value)) return "名称不能包含换行或控制字符";
  if (duplicateAlias.value) return "当前存档中已有同名据点";
  return "";
});

async function load() {
  loading.value = true;
  const requests = [api.getBaseCamps()];
  if (isAdmin.value) requests.push(api.getBaseAliases());
  const [baseResponse, aliasResponse] = await Promise.all(requests);
  loading.value = false;
  if (baseResponse.statusCode.value !== 200) {
    message.error(baseResponse.data.value?.error || "据点数据读取失败");
    return;
  }
  bases.value = baseResponse.data.value?.items || [];
  metadata.value = baseResponse.data.value?.metadata || metadata.value;
  aliases.value = aliasResponse?.statusCode.value === 200 ? aliasResponse.data.value?.items || [] : [];
}

function openEditor(base) {
  editingBase.value = base;
  aliasName.value = base.custom_name || "";
  editorOpen.value = true;
}

async function saveAlias() {
  if (saving.value || aliasError.value || !editingBase.value) return;
  saving.value = true;
  const { data, statusCode } = await api.updateBaseAlias(editingBase.value.base_id, { name: trimmedAlias.value });
  saving.value = false;
  if (statusCode.value !== 200) {
    message.error(data.value?.error || "据点名称保存失败");
    return;
  }
  editorOpen.value = false;
  message.success("据点名称已更新");
  await load();
}

function resetAlias(base) {
  dialog.warning({
    title: "重置据点名称",
    content: `确定清除“${base.display_name}”的自定义名称吗？`,
    positiveText: "重置",
    negativeText: "取消",
    onPositiveClick: async () => {
      const { data, statusCode } = await api.deleteBaseAlias(base.base_id);
      if (statusCode.value !== 200) message.error(data.value?.error || "重置失败");
      else { message.success("已恢复存档名称"); await load(); }
    },
  });
}

function deleteInactive(item) {
  dialog.warning({
    title: "清理失效名称",
    content: `“${item.display_name}”已不属于当前存档。删除后无法恢复，是否继续？`,
    positiveText: "删除记录",
    negativeText: "取消",
    onPositiveClick: async () => {
      const { data, statusCode } = await api.deleteBaseAlias(item.base_id);
      if (statusCode.value !== 200) message.error(data.value?.error || "清理失败");
      else { message.success("失效名称已清理"); await load(); }
    },
  });
}

onMounted(load);
</script>

<template>
  <operations-shell
    title="据点管理"
    subtitle="为每个据点设置清晰、稳定的中文名称。名称仅保存在 PST 中，不会修改 Palworld 世界存档。"
    :metadata="metadata"
    :loading="loading"
    @refresh="load"
  >
    <n-alert v-if="!isAdmin" type="info" :bordered="false" class="notice">
      当前以访客身份浏览。登录管理员账号后可以修改据点名称。
    </n-alert>
    <n-alert v-for="warning in metadata.warnings || []" :key="warning" type="warning" class="notice">{{ warning }}</n-alert>

    <n-spin :show="loading">
      <section v-if="bases.length" class="base-grid">
        <article v-for="base in bases" :key="base.base_id" class="base-card">
          <header>
            <div>
              <span class="base-kicker">据点 {{ shortID(base.base_id) }}</span>
              <h2>{{ base.display_name }}</h2>
            </div>
            <n-tag v-if="base.custom_name" type="success" size="small" :bordered="false">自定义名称</n-tag>
            <n-tag v-else size="small" :bordered="false">存档名称</n-tag>
          </header>

          <dl class="base-metrics">
            <div><dt>工作帕鲁</dt><dd>{{ base.worker_pal_count }}/{{ base.max_worker_pals }}</dd></div>
            <div><dt>需要关注</dt><dd :class="{ warning: attentionCount(base) }">{{ attentionCount(base) }}</dd></div>
            <div><dt>饲料库存</dt><dd>{{ Number(base.feed_total_item_count || 0).toLocaleString() }}</dd></div>
          </dl>

          <div class="base-details">
            <span><small>公会</small>{{ base.guild_name || "未识别" }}</span>
            <span><small>坐标</small>{{ coordinate(base) }}</span>
            <span v-if="isAdmin"><small>存档原名</small>{{ base.base_name || "无" }}</span>
          </div>

          <footer v-if="isAdmin">
            <n-button type="primary" secondary @click="openEditor(base)">{{ base.custom_name ? "修改名称" : "设置名称" }}</n-button>
            <n-button v-if="base.custom_name" text type="error" @click="resetAlias(base)">恢复存档名称</n-button>
          </footer>
        </article>
      </section>
      <n-empty v-else-if="!loading" description="尚未解析到据点，请先完成一次存档同步" class="empty" />
    </n-spin>

    <n-collapse v-if="isAdmin && inactiveAliases.length" class="inactive-panel">
      <n-collapse-item :title="`失效名称记录（${inactiveAliases.length}）`" name="inactive">
        <p class="inactive-help">这些据点已不在当前存档中，可能来自旧世界或已经拆除的据点。系统不会自动删除。</p>
        <n-list bordered>
          <n-list-item v-for="item in inactiveAliases" :key="item.base_id">
            <n-thing :title="item.display_name" :description="`据点 ID：${item.base_id} · 更新于 ${new Date(item.updated_at).toLocaleString('zh-CN')}`" />
            <template #suffix><n-button size="small" type="error" secondary @click="deleteInactive(item)">清理</n-button></template>
          </n-list-item>
        </n-list>
      </n-collapse-item>
    </n-collapse>

    <n-drawer v-model:show="editorOpen" :width="420" placement="right">
      <n-drawer-content title="设置据点名称" closable>
        <div v-if="editingBase" class="editor-summary">
          <span>当前显示</span><strong>{{ editingBase.display_name }}</strong>
          <small>据点 ID：{{ editingBase.base_id }}</small>
        </div>
        <n-form-item label="自定义名称" :validation-status="aliasError ? 'error' : undefined" :feedback="aliasError || '名称将在工作帕鲁、库存、配种农场和游戏内提醒中统一显示'">
          <n-input v-model:value="aliasName" maxlength="40" show-count clearable placeholder="例如：北境矿业基地" @keyup.enter="saveAlias" />
        </n-form-item>
        <template #footer>
          <n-space justify="end">
            <n-button @click="editorOpen = false">取消</n-button>
            <n-button type="primary" :disabled="Boolean(aliasError)" :loading="saving" @click="saveAlias">保存名称</n-button>
          </n-space>
        </template>
      </n-drawer-content>
    </n-drawer>
  </operations-shell>
</template>

<style scoped>
.notice { margin-bottom: 12px; }
.base-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(310px, 1fr)); gap: 15px; }
.base-card { display: flex; flex-direction: column; min-height: 305px; padding: 20px; border: 1px solid var(--ops-line); border-radius: 15px; background: var(--ops-panel); box-shadow: 0 14px 32px rgba(37, 67, 56, .045); transition: transform .22s, box-shadow .22s, border-color .22s; }
.base-card:hover { transform: translateY(-2px); border-color: rgba(47, 125, 104, .28); box-shadow: 0 18px 38px rgba(37, 67, 56, .08); }
.base-card header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.base-kicker { color: var(--ops-muted); font: 500 10px/1.4 ui-monospace, monospace; letter-spacing: .06em; }
h2 { margin: 5px 0 0; font-size: 22px; line-height: 1.2; letter-spacing: -.025em; }
.base-metrics { display: grid; grid-template-columns: repeat(3, 1fr); gap: 7px; margin: 20px 0; }
.base-metrics div { padding: 11px; border-radius: 9px; background: rgba(47, 125, 104, .065); }
dt { color: var(--ops-muted); font-size: 11px; }
dd { margin: 5px 0 0; font: 650 20px/1 ui-monospace, monospace; font-variant-numeric: tabular-nums; }
dd.warning { color: #b66f1f; }
.base-details { display: grid; gap: 8px; color: var(--ops-text); font-size: 13px; }
.base-details span { display: grid; grid-template-columns: 72px minmax(0, 1fr); gap: 8px; }
.base-details small { color: var(--ops-muted); }
.base-card footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: auto; padding-top: 20px; }
.empty { padding: 70px 0; }
.inactive-panel { margin-top: 20px; padding: 5px 14px; border: 1px solid var(--ops-line); border-radius: 12px; background: var(--ops-panel); }
.inactive-help { margin: 0 0 12px; color: var(--ops-muted); font-size: 13px; }
.editor-summary { margin-bottom: 24px; padding: 16px; border-radius: 11px; background: rgba(47, 125, 104, .08); }
.editor-summary span, .editor-summary strong, .editor-summary small { display: block; }
.editor-summary span, .editor-summary small { color: var(--ops-muted); font-size: 12px; }
.editor-summary strong { margin: 5px 0; font-size: 20px; }
@media (max-width: 640px) {
  .base-grid { grid-template-columns: 1fr; }
  .base-card { min-height: 0; padding: 16px; }
  .base-metrics div { padding: 9px; }
  dd { font-size: 18px; }
  :deep(.n-drawer) { width: 100% !important; }
}
</style>
