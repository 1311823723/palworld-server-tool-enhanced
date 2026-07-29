<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { useDialog, useMessage } from "naive-ui";
import ApiService from "@/service/api";
import OperationsShell from "@/components/OperationsShell.vue";
import itemsMap from "@/assets/items.json";

const api = new ApiService();
const message = useMessage();
const dialog = useDialog();
const loading = ref(false);
const action = ref("");
const bridge = ref({
  state: "unconfigured",
  message: "正在检测 Production Bridge",
  bundled_version: "0.1.0",
  orders_available: false,
  catalog_available: false,
});
const catalog = ref({ bases: [] });
const orders = ref([]);
const preview = ref(null);
const installOpen = ref(false);
const installAction = ref("install");
const installForm = reactive({
  confirmation: "",
  shutdown_seconds: 30,
  restart_delay_seconds: 10,
  message: "服务器将在 30 秒后安装生产 Bridge，请提前回到安全位置。",
});
const orderForm = reactive({
  base_id: "",
  workstation_actor_guid: "",
  recipe_id: "",
  quantity_mode: "exact",
  quantity: 1,
});
let pollTimer;

const localeItems = computed(() => itemsMap.zh || []);
const itemByID = computed(() => Object.fromEntries(localeItems.value.flatMap((item) => [
  [String(item.key || "").toLowerCase(), item],
  [String(item.id || "").toLowerCase(), item],
])));
const itemLabel = (id, fallback = "") => itemByID.value[String(id || "").toLowerCase()]?.name || fallback || `未知物品（${id || "无 ID"}）`;

const statePresentation = computed(() => ({
  unsupported: ["不支持", "default"],
  unconfigured: ["未配置", "warning"],
  dependency_missing: ["缺少 UE4SS", "warning"],
  not_installed: ["未安装", "warning"],
  installing: ["正在安装", "info"],
  restart_required: ["等待重启", "warning"],
  healthy: ["运行正常", "success"],
  offline: ["Bridge 离线", "error"],
  upgrade_available: ["可升级", "warning"],
  modified: ["文件已修改", "error"],
  incompatible: ["版本不兼容", "error"],
  permission_denied: ["权限不足", "error"],
  error: ["检测失败", "error"],
}[bridge.value.state] || [bridge.value.state, "default"]));

const bases = computed(() => catalog.value?.bases || []);
const baseOptions = computed(() => bases.value.map((base) => ({
  label: base.base_name || `未命名据点（${base.base_id.slice(-6)}）`,
  value: base.base_id,
})));
const selectedBase = computed(() => bases.value.find((base) => base.base_id === orderForm.base_id));
const stations = computed(() => selectedBase.value?.workstations || []);
const stationOptions = computed(() => stations.value.map((station) => ({
  label: station.name || `工作台 ${station.actor_guid.slice(-6)}`,
  value: station.actor_guid,
  disabled: station.busy,
})));
const selectedStation = computed(() => stations.value.find((station) => station.actor_guid === orderForm.workstation_actor_guid));
const recipes = computed(() => selectedStation.value?.recipes || []);
const recipeOptions = computed(() => recipes.value.map((recipe) => ({
  label: `${itemLabel(recipe.product_item_id, recipe.product_name || recipe.name)} × ${recipe.product_count || 1}`,
  value: recipe.id,
  disabled: !recipe.unlocked,
})));
const selectedRecipe = computed(() => recipes.value.find((recipe) => recipe.id === orderForm.recipe_id));
const canPreview = computed(() => orderForm.base_id && orderForm.workstation_actor_guid && orderForm.recipe_id
  && (orderForm.quantity_mode === "max_available" || Number(orderForm.quantity) > 0));
const canSubmit = computed(() => bridge.value.orders_available && preview.value?.can_submit && !action.value);
const canAutomaticInstall = computed(() => bridge.value.manual_install?.automatic_install_available);
const canInstall = computed(() => ["not_installed", "upgrade_available", "restart_required", "offline"].includes(bridge.value.state) && canAutomaticInstall.value);
const canRepair = computed(() => bridge.value.state === "modified" && canAutomaticInstall.value);
const canDisable = computed(() => bridge.value.installed_version && !["not_installed", "unconfigured", "unsupported"].includes(bridge.value.state));
const installPercentage = computed(() => ({
  save: 12,
  backup: 28,
  shutdown: 46,
  install: 68,
  start: 84,
  health: 94,
}[bridge.value.install_stage] || 8));
const installButtonLabel = computed(() => ({
  upgrade_available: "升级 Bridge",
  restart_required: "启用 Bridge",
  offline: "重新安装 Bridge",
}[bridge.value.state] || "一键安装"));

const orderStatus = (value) => ({
  pending: ["等待派发", "default"],
  dispatching: ["正在派发", "info"],
  accepted: ["游戏已接受", "success"],
  waiting_materials: ["等待材料", "warning"],
  producing: ["生产中", "info"],
  completed: ["已完成", "success"],
  cancelled: ["已取消", "default"],
  failed: ["失败", "error"],
  unknown: ["状态未知", "warning"],
}[value] || [value, "default"]);
const canCancel = (order) => ["pending", "dispatching", "waiting_materials"].includes(order.status) && !order.cancellation_requested;
const shortID = (value) => value ? `${value.slice(0, 8)}…` : "—";
const formatTime = (value) => value ? new Date(value).toLocaleString("zh-CN") : "—";

async function loadBridge(silent = false) {
  if (!silent) loading.value = true;
  const { data, statusCode } = await api.getProductionBridge();
  if (!silent) loading.value = false;
  if (Number(statusCode.value) !== 200) {
    if (!silent) message.error(data.value?.error || "Bridge 状态读取失败");
    return;
  }
  bridge.value = data.value || bridge.value;
}

async function recheckBridge() {
  if (action.value) return;
  action.value = "bridge-recheck";
  const { data, statusCode } = await api.recheckProductionBridge();
  action.value = "";
  if (Number(statusCode.value) !== 200) {
    message.error(data.value?.error || "Bridge 重新检测失败");
    return;
  }
  bridge.value = data.value || bridge.value;
  await Promise.all([loadCatalog(true), loadOrders(true)]);
  message.success(`检测完成：${bridge.value.message || "状态已更新"}`);
}

async function loadCatalog(silent = false) {
  if (!bridge.value.catalog_available && bridge.value.state !== "healthy") return;
  const { data, statusCode } = await api.getProductionCatalog();
  if (Number(statusCode.value) === 200) catalog.value = data.value || { bases: [] };
  else if (!silent) message.error(data.value?.error || "生产目录读取失败");
}

async function loadOrders(silent = false) {
  const { data, statusCode } = await api.getProductionOrders({ limit: 200 });
  if (Number(statusCode.value) === 200) orders.value = data.value?.items || [];
  else if (!silent) message.error(data.value?.error || "订单历史读取失败");
}

async function load(silent = false) {
  await loadBridge(silent);
  await Promise.all([loadCatalog(silent), loadOrders(silent)]);
}

function openMaintenance(kind) {
  installAction.value = kind;
  installForm.confirmation = "";
  installForm.message = kind === "disable"
    ? "服务器将在 30 秒后安全禁用生产 Bridge，请提前回到安全位置。"
    : "服务器将在 30 秒后安装生产 Bridge，请提前回到安全位置。";
  installOpen.value = true;
}

async function submitMaintenance() {
  if (installForm.confirmation !== "INSTALL" || action.value) return;
  action.value = `bridge-${installAction.value}`;
  const method = {
    install: "installProductionBridge",
    repair: "repairProductionBridge",
    disable: "disableProductionBridge",
  }[installAction.value];
  const { data, statusCode } = await api[method]({ ...installForm });
  action.value = "";
  if (Number(statusCode.value) !== 202) {
    message.error(data.value?.error || "Bridge 维护流程启动失败");
    return;
  }
  installOpen.value = false;
  message.success("Bridge 安全维护流程已启动");
  bridge.value = data.value || bridge.value;
}

async function copyText(value) {
  try {
    await navigator.clipboard.writeText(value);
    message.success("路径已复制");
  } catch {
    message.warning("浏览器未允许复制，请手动选择路径");
  }
}

async function runPreview() {
  if (!canPreview.value || action.value) return;
  action.value = "preview";
  const { data, statusCode } = await api.previewProductionOrder({ ...orderForm, quantity: Number(orderForm.quantity || 0) });
  action.value = "";
  if (Number(statusCode.value) !== 200) {
    preview.value = null;
    message.error(data.value?.error || "材料预览失败");
    return;
  }
  preview.value = data.value;
}

async function submitOrder() {
  if (!canSubmit.value) return;
  action.value = "create-order";
  const { data, statusCode } = await api.createProductionOrder({ ...orderForm, quantity: Number(orderForm.quantity || 0) });
  action.value = "";
  if (Number(statusCode.value) !== 201) {
    message.error(data.value?.error || "生产订单提交失败");
    return;
  }
  message.success(`订单 ${shortID(data.value?.order_id)} 已派发`);
  preview.value = null;
  await loadOrders();
}

function cancelOrder(order) {
  dialog.warning({
    title: "取消生产订单",
    content: "仅当游戏尚未接受订单时可以取消。若工作台已经开始生产，PST 不会强制清除游戏队列。",
    positiveText: "请求取消",
    negativeText: "返回",
    onPositiveClick: async () => {
      action.value = `cancel-${order.order_id}`;
      const { data, statusCode } = await api.cancelProductionOrder(order.order_id);
      action.value = "";
      if (Number(statusCode.value) !== 200) message.error(data.value?.error || "订单取消失败");
      else {
        message.success("取消请求已发送给 Bridge");
        await loadOrders();
      }
    },
  });
}

watch(() => orderForm.base_id, () => {
  orderForm.workstation_actor_guid = "";
  orderForm.recipe_id = "";
  preview.value = null;
});
watch(() => orderForm.workstation_actor_guid, () => {
  orderForm.recipe_id = "";
  preview.value = null;
});
watch([() => orderForm.recipe_id, () => orderForm.quantity_mode, () => orderForm.quantity], () => {
  preview.value = null;
});

onMounted(async () => {
  await load();
  pollTimer = window.setInterval(() => load(true), 2500);
});
onBeforeUnmount(() => window.clearInterval(pollTimer));
</script>

<template>
  <operations-shell
    title="生产订单"
    subtitle="通过本机 Production Bridge 向指定据点工作台安排生产；不修改存档，日常下单无需停服。"
    :loading="loading"
    @refresh="load"
  >
    <section class="bridge-panel">
      <header class="panel-heading">
        <div>
          <h2>PST Production Bridge</h2>
          <p>{{ bridge.message }}</p>
        </div>
        <n-tag :type="statePresentation[1]" :bordered="false" round>{{ statePresentation[0] }}</n-tag>
      </header>
      <div class="bridge-facts">
        <div><span>已安装版本</span><strong>{{ bridge.installed_version || "—" }}</strong></div>
        <div><span>随包版本</span><strong>{{ bridge.bundled_version }}</strong></div>
        <div><span>运行时心跳</span><strong>{{ formatTime(bridge.heartbeat_at) }}</strong></div>
        <div><span>Palworld Build</span><strong>{{ bridge.palworld_build || "未报告" }}</strong></div>
      </div>
      <n-progress
        v-if="bridge.installing"
        type="line"
        processing
        :percentage="installPercentage"
        :show-indicator="false"
        status="success"
        class="install-progress"
      />
      <n-alert v-if="bridge.last_error" type="error" class="bridge-alert">{{ bridge.last_error }}</n-alert>
      <div class="bridge-actions">
        <n-button v-if="canInstall" type="primary" @click="openMaintenance('install')">{{ installButtonLabel }}</n-button>
        <n-button v-if="canRepair" type="warning" @click="openMaintenance('repair')">修复已修改文件</n-button>
        <n-button v-if="canDisable" secondary type="error" @click="openMaintenance('disable')">安全禁用</n-button>
        <n-button :loading="action === 'bridge-recheck'" :disabled="Boolean(action && action !== 'bridge-recheck')" secondary @click="recheckBridge">重新检测</n-button>
      </div>
    </section>

    <n-alert v-if="bridge.state === 'dependency_missing'" type="warning" class="section-gap">
      PST 不会自动安装、升级或覆盖 UE4SS。请先人工安装与当前 Palworld Build 兼容的 UE4SS，再返回此页检测。
    </n-alert>

    <n-collapse v-if="bridge.manual_install" class="manual-panel section-gap">
      <n-collapse-item title="查看人工安装路径与步骤" name="manual">
        <div class="path-list">
          <div v-for="entry in [
            ['Release 安装包', bridge.manual_install.source_directory],
            ['Bridge 目标目录', bridge.manual_install.target_directory],
            ['PalModSettings.ini', bridge.manual_install.settings_path],
            ['UE4SS 预期目录', bridge.manual_install.ue4ss_directory],
          ]" :key="entry[0]">
            <span>{{ entry[0] }}</span>
            <code>{{ entry[1] || "尚未推导" }}</code>
            <n-button v-if="entry[1]" size="tiny" text type="primary" @click="copyText(entry[1])">复制</n-button>
          </div>
        </div>
        <ol class="manual-steps">
          <li v-for="step in bridge.manual_install.steps || []" :key="step">{{ step }}</li>
        </ol>
      </n-collapse-item>
    </n-collapse>

    <section v-if="bridge.orders_available" class="order-workspace section-gap">
      <div class="order-form">
        <header class="panel-heading">
          <div><h2>创建生产订单</h2><p>选择据点、具体工作台和配方。提交时 Bridge 会再次校验归属、材料和队列。</p></div>
        </header>
        <div class="form-grid">
          <n-form-item label="据点">
            <n-select v-model:value="orderForm.base_id" filterable :options="baseOptions" placeholder="选择据点" />
          </n-form-item>
          <n-form-item label="工作台">
            <n-select v-model:value="orderForm.workstation_actor_guid" filterable :options="stationOptions" :disabled="!orderForm.base_id" placeholder="选择具体工作台" />
          </n-form-item>
          <n-form-item label="配方">
            <n-select v-model:value="orderForm.recipe_id" filterable :options="recipeOptions" :disabled="!orderForm.workstation_actor_guid" placeholder="选择配方" />
          </n-form-item>
          <n-form-item label="数量模式">
            <n-radio-group v-model:value="orderForm.quantity_mode">
              <n-space><n-radio value="exact">指定数量</n-radio><n-radio value="max_available">按实时材料最大生产</n-radio></n-space>
            </n-radio-group>
          </n-form-item>
          <n-form-item v-if="orderForm.quantity_mode === 'exact'" label="生产数量">
            <n-input-number v-model:value="orderForm.quantity" :min="1" :max="999999" />
          </n-form-item>
        </div>
        <div class="form-actions">
          <n-button :disabled="!canPreview" :loading="action === 'preview'" secondary @click="runPreview">检查实时材料</n-button>
          <n-button type="primary" :disabled="!canSubmit" :loading="action === 'create-order'" @click="submitOrder">提交生产订单</n-button>
        </div>
      </div>

      <aside class="preview-panel">
        <template v-if="preview">
          <header>
            <span>实时材料预览</span>
            <strong>最多可生产 {{ preview.max_available }}</strong>
          </header>
          <div class="preview-quantity">
            <span>本次接受数量</span><strong>{{ preview.accepted_quantity }}</strong>
          </div>
          <div v-for="material in preview.materials || []" :key="material.item_id" class="material-row">
            <div><strong>{{ itemLabel(material.item_id, material.name) }}</strong><small>{{ material.item_id }}</small></div>
            <div><span>每件 {{ material.required_each }}</span><b :class="{ shortage: material.shortage }">{{ material.available }} / {{ material.required }}</b></div>
          </div>
          <n-alert v-if="!preview.can_submit" type="warning" :bordered="false">{{ preview.reason || "当前无法提交" }}</n-alert>
        </template>
        <n-empty v-else description="选择配方后检查材料，预览不会预留库存" />
      </aside>
    </section>

    <n-alert v-else-if="bridge.state === 'incompatible'" type="error" class="section-gap">
      Bridge 已加载，但当前 Palworld Build 的生产适配器未通过能力验证。PST 已停止接收订单，其他服务器功能不受影响。
    </n-alert>

    <section class="orders-panel section-gap">
      <header class="panel-heading sticky-heading">
        <div><h2>订单队列</h2><p>游戏接受后的订单不会被 PST 强制取消；状态无法可靠恢复时会标记为“状态未知”。</p></div>
        <n-button size="small" secondary @click="loadOrders">刷新</n-button>
      </header>
      <div v-if="orders.length" class="order-list">
        <article v-for="order in orders" :key="order.order_id" class="order-row">
          <div class="order-product">
            <strong>{{ itemLabel(order.product_item_id, order.product_name || order.recipe_name) }}</strong>
            <small>{{ order.base_name || order.base_id }} · {{ order.workstation_name || shortID(order.workstation_actor_guid) }}</small>
          </div>
          <div class="order-progress">
            <span>{{ order.completed_quantity || 0 }} / {{ order.accepted_quantity || order.requested_quantity || 0 }}</span>
            <n-progress type="line" :show-indicator="false" :percentage="Math.min(100, Math.round((order.completed_quantity || 0) / Math.max(1, order.accepted_quantity || order.requested_quantity || 1) * 100))" />
          </div>
          <div class="order-state">
            <n-tag :type="orderStatus(order.status)[1]" size="small" :bordered="false">{{ orderStatus(order.status)[0] }}</n-tag>
            <small>{{ formatTime(order.updated_at) }}</small>
          </div>
          <n-button v-if="canCancel(order)" size="small" secondary type="error" :loading="action === `cancel-${order.order_id}`" @click="cancelOrder(order)">取消</n-button>
          <span v-else-if="order.cancellation_requested" class="cancel-note">取消确认中</span>
          <p v-if="order.error" class="order-error">{{ order.error }}</p>
        </article>
      </div>
      <n-empty v-else description="还没有生产订单" class="empty-orders" />
    </section>

    <n-modal v-model:show="installOpen" :mask-closable="!action">
      <n-card class="install-dialog" :title="installAction === 'disable' ? '安全禁用 Production Bridge' : installAction === 'repair' ? '修复 Production Bridge' : '安装 Production Bridge'" :bordered="false" role="dialog">
        <n-alert type="warning" :bordered="false">
          运行中的服务器将依次执行：保存世界 → 创建备份 → 平滑停服 → 等待退出 → 维护 Bridge → 启动并检查心跳。已停止服务器维护后会保持停止。
        </n-alert>
        <div class="dialog-grid">
          <n-form-item label="关服倒计时（秒）"><n-input-number v-model:value="installForm.shutdown_seconds" :min="0" :max="600" /></n-form-item>
          <n-form-item label="重启等待（秒）"><n-input-number v-model:value="installForm.restart_delay_seconds" :min="0" :max="600" /></n-form-item>
        </div>
        <n-form-item label="游戏内广播"><n-input v-model:value="installForm.message" maxlength="200" show-count /></n-form-item>
        <n-form-item label="确认操作">
          <n-input v-model:value="installForm.confirmation" placeholder="输入 INSTALL" @keyup.enter="submitMaintenance" />
        </n-form-item>
        <div class="dialog-actions">
          <n-button :disabled="Boolean(action)" @click="installOpen = false">取消</n-button>
          <n-button type="primary" :disabled="installForm.confirmation !== 'INSTALL'" :loading="Boolean(action)" @click="submitMaintenance">确认执行</n-button>
        </div>
      </n-card>
    </n-modal>
  </operations-shell>
</template>

<style scoped>
.section-gap { margin-top: 16px; }
.bridge-panel, .order-form, .preview-panel, .orders-panel, .manual-panel {
  border: 1px solid var(--ops-line);
  border-radius: 13px;
  background: var(--ops-panel);
  box-shadow: 0 12px 28px rgba(37, 67, 56, .045);
}
.bridge-panel { padding: 20px; }
.panel-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
.panel-heading h2 { margin: 0; font-size: 19px; letter-spacing: -.02em; }
.panel-heading p { margin: 5px 0 0; color: var(--ops-muted); font-size: 13px; line-height: 1.55; }
.bridge-facts { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 1px; margin-top: 18px; border: 1px solid var(--ops-line); background: var(--ops-line); }
.bridge-facts div { min-width: 0; padding: 14px; background: var(--ops-panel); }
.bridge-facts span, .bridge-facts strong { display: block; }
.bridge-facts span { color: var(--ops-muted); font-size: 11px; }
.bridge-facts strong { overflow: hidden; margin-top: 5px; font: 650 14px/1.3 ui-monospace, monospace; text-overflow: ellipsis; white-space: nowrap; }
.bridge-actions, .form-actions, .dialog-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 9px; margin-top: 18px; }
.bridge-alert, .install-progress { margin-top: 14px; }
.manual-panel { padding: 3px 14px; }
.path-list { display: grid; gap: 8px; }
.path-list > div { display: grid; grid-template-columns: 130px minmax(0, 1fr) auto; align-items: center; gap: 10px; padding: 9px 11px; background: rgba(47, 125, 104, .055); }
.path-list span { color: var(--ops-muted); font-size: 12px; }
.path-list code { overflow: hidden; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.manual-steps { margin: 16px 0 4px; padding-left: 22px; color: var(--ops-muted); font-size: 13px; line-height: 1.8; }
.order-workspace { display: grid; grid-template-columns: minmax(0, 1.6fr) minmax(300px, .8fr); gap: 16px; }
.order-form, .preview-panel { padding: 20px; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); column-gap: 14px; margin-top: 15px; }
.preview-panel { min-height: 330px; }
.preview-panel header, .preview-quantity { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
.preview-panel header span, .preview-quantity span { color: var(--ops-muted); font-size: 12px; }
.preview-panel header strong { font-size: 14px; }
.preview-quantity { margin: 18px 0; padding: 14px; background: rgba(47, 125, 104, .075); }
.preview-quantity strong { color: #178d79; font: 700 27px/1 ui-monospace, monospace; }
.material-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 14px; padding: 11px 0; border-top: 1px solid var(--ops-line); }
.material-row > div:last-child { text-align: right; }
.material-row strong, .material-row small, .material-row span, .material-row b { display: block; }
.material-row small, .material-row span { margin-top: 3px; color: var(--ops-muted); font-size: 11px; }
.material-row b { margin-top: 3px; font: 650 13px/1 ui-monospace, monospace; }
.material-row b.shortage { color: #c14e42; }
.orders-panel { overflow: clip; }
.sticky-heading { position: sticky; top: 0; z-index: 2; padding: 17px 20px; border-bottom: 1px solid var(--ops-line); background: var(--ops-panel); }
.order-list { max-height: 560px; overflow-y: auto; overscroll-behavior: contain; }
.order-row { position: relative; display: grid; grid-template-columns: minmax(180px, 1.4fr) minmax(150px, .8fr) 130px auto; align-items: center; gap: 16px; min-height: 82px; padding: 14px 20px; border-bottom: 1px solid var(--ops-line); }
.order-product strong, .order-product small, .order-state small { display: block; }
.order-product small, .order-state small { margin-top: 4px; color: var(--ops-muted); font-size: 11px; }
.order-progress span { font: 600 12px/1 ui-monospace, monospace; }
.order-progress .n-progress { margin-top: 7px; }
.order-error { grid-column: 1 / -1; margin: -7px 0 0; color: #c14e42; font-size: 12px; }
.cancel-note { color: var(--ops-muted); font-size: 12px; }
.empty-orders { padding: 54px 0; }
.install-dialog { width: min(620px, calc(100vw - 28px)); }
.dialog-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; margin-top: 18px; }
@media (max-width: 900px) {
  .bridge-facts { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .order-workspace { grid-template-columns: 1fr; }
  .preview-panel { min-height: 0; }
}
@media (max-width: 640px) {
  .bridge-panel, .order-form, .preview-panel { padding: 15px; }
  .bridge-facts { grid-template-columns: 1fr 1fr; }
  .bridge-facts div { padding: 11px; }
  .form-grid, .dialog-grid { grid-template-columns: 1fr; }
  .path-list > div { grid-template-columns: 1fr auto; }
  .path-list span { grid-column: 1 / -1; }
  .path-list code { white-space: normal; word-break: break-all; }
  .order-row { grid-template-columns: minmax(0, 1fr) auto; gap: 10px; padding: 13px 15px; }
  .order-progress { grid-column: 1 / -1; grid-row: 2; }
  .order-state { grid-column: 2; grid-row: 1; text-align: right; }
  .order-row > .n-button, .cancel-note { grid-column: 1 / -1; justify-self: start; }
}
</style>
