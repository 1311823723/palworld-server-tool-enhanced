<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useMessage } from "naive-ui";
import { useRoute, useRouter } from "vue-router";
import ApiService from "@/service/api";
import OperationsShell from "@/components/OperationsShell.vue";
import pageStore from "@/stores/model/page";
import { breedingCapabilitiesReliable, unseenBreedingEvents } from "@/utils/enhancedViews";
import palMap from "@/assets/pal.json";
import { localizedPalName } from "@/utils/gameLabels";

const api = new ApiService();
const message = useMessage();
const route = useRoute();
const router = useRouter();
const { t, locale } = useI18n();
const loading = ref(false);
const saving = ref(false);
const farms = ref([]);
const metadata = ref({ warnings: [] });
const parserStatus = ref({ failed: false });
const capabilities = ref({});
const events = ref([]);
const unreadEvents = ref([]);
const unreadTotal = ref(0);
const showSettings = ref(false);
const showNotifications = ref(false);
const confirmAll = ref(false);
const isMobile = computed(() => pageStore().getScreenWidth() < 768);
const filter = reactive({ base_id: null, guild_id: null, has_egg: false, cake_empty: false, parent_missing: false, has_warning: false, sort: "egg_count" });
const settings = reactive({ enabled: false, selection_mode: "selected", selected_base_ids: [], selected_farm_ids: [], notify_existing_on_enable: false, notify_on_each_egg: true, minimum_ready_eggs: 1, browser_notifications: true, in_app_notifications: true, game_notifications: true, game_notification_message: "【配种提醒】据点「{base}」有 {new_count} 枚新蛋可以拾取，当前共有 {count} 枚。", history_retention_days: 30 });
const reliable = computed(() => breedingCapabilitiesReliable(capabilities.value));
let pollTimer;

const seenStorageKey = "pst_breeding_seen_events";
function loadSeenIDs() {
  try { return new Set(JSON.parse(localStorage.getItem(seenStorageKey) || "[]")); }
  catch { return new Set(); }
}
const seenEventIDs = loadSeenIDs();
function persistSeenIDs() {
  localStorage.setItem(seenStorageKey, JSON.stringify([...seenEventIDs].slice(-500)));
}

const errorText = (data) => data?.error || t("breeding.error");
const count = (value) => value == null ? t("breeding.unknown") : Number(value).toLocaleString();
const shortID = (value) => value ? `${value.slice(0, 8)}…${value.slice(-4)}` : t("breeding.unknown");
const formatDate = (value) => value ? new Intl.DateTimeFormat(locale.value, { dateStyle: "medium", timeStyle: "medium" }).format(new Date(value)) : t("breeding.unknown");
const relativeAge = (value) => value ? t("breeding.ageSeconds", { seconds: Math.max(0, Math.round((Date.now() - new Date(value).getTime()) / 1000)) }) : t("breeding.unknown");
const displayPal = (id) => localizedPalName(id, palMap, "zh");
const statusLabel = (status) => t(`breeding.status.${status || "unknown"}`);
const statusType = (status) => ({ egg_ready: "success", parent_missing: "warning", cake_empty: "error", unsupported: "default" }[status] || "info");
const genderLabel = (gender) => ({ Male: "♂", Female: "♀" }[gender] || "—");

const baseName = (value) => value.base_display_name || value.base_name || shortID(value.base_id);
const baseOptions = computed(() => [...new Map(farms.value.map((farm) => [farm.base_id, { label: baseName(farm), value: farm.base_id }])).values()]);
const guildOptions = computed(() => [...new Map(farms.value.map((farm) => [farm.guild_id, { label: farm.guild_name || shortID(farm.guild_id), value: farm.guild_id }])).values()]);
const farmOptions = computed(() => farms.value.map((farm) => ({ label: `${baseName(farm)} · ${shortID(farm.farm_id)}`, value: farm.farm_id })));
const groupedFarms = computed(() => {
  const groups = new Map();
  for (const farm of farms.value) {
    const key = farm.base_id || "unknown";
    if (!groups.has(key)) groups.set(key, { baseID: key, baseName: baseName(farm), guildName: farm.guild_name || t("breeding.unknown"), farms: [], eggTotal: 0, cakeTotal: 0, abnormal: 0 });
    const group = groups.get(key);
    group.farms.push(farm);
    group.eggTotal += Number(farm.egg_count || 0);
    group.cakeTotal += Number(farm.cake_count || 0);
    if (farm.warnings?.length || !farm.parsing_complete || ["parent_missing", "cake_empty", "unsupported"].includes(farm.status)) group.abnormal += 1;
  }
  return [...groups.values()];
});

function farmMonitored(farm) {
  if (!settings.enabled) return false;
  if (settings.selection_mode === "all") return true;
  return settings.selected_farm_ids?.includes(farm.farm_id) || settings.selected_base_ids?.includes(farm.base_id);
}

function farmQuery() {
  return {
    page_size: 200,
    base_id: filter.base_id,
    guild_id: filter.guild_id,
    has_egg: filter.has_egg || "",
    cake_empty: filter.cake_empty || "",
    parent_missing: filter.parent_missing || "",
    has_warning: filter.has_warning || "",
    sort: filter.sort,
  };
}

async function loadFarms() {
  loading.value = true;
  const { data, statusCode } = await api.getBreedingFarms(farmQuery());
  loading.value = false;
  if (statusCode.value !== 200) { message.error(errorText(data.value)); return; }
  farms.value = data.value?.items || [];
  metadata.value = data.value?.metadata || metadata.value;
  parserStatus.value = data.value?.parser_status || parserStatus.value;
  await nextTick();
  const requestedFarm = route.query.farm;
  if (requestedFarm) document.getElementById(`farm-${requestedFarm}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
}

async function loadCapabilities() {
  const { data, statusCode } = await api.getBreedingCapabilities();
  if (statusCode.value === 200) capabilities.value = data.value?.capabilities || {};
}

async function loadSettings() {
  const { data, statusCode } = await api.getBreedingNotificationConfig();
  if (statusCode.value !== 200) return;
  Object.assign(settings, data.value || {});
}

function notifyBrowser(event) {
  if (!settings.browser_notifications || typeof Notification === "undefined" || Notification.permission !== "granted") return;
  const notification = new Notification(t("breeding.browserTitle"), {
    body: t("breeding.browserBody", { base: event.base_display_name || event.base_name || t("breeding.unknown"), previous: event.previous_count, current: event.current_count }),
    tag: `pst-breeding-${event.event_id}`,
  });
  notification.onclick = () => {
    window.focus();
    router.push({ path: "/breeding-farms", query: { farm: event.farm_id } });
    notification.close();
  };
}

async function loadUnreadEvents({ announce = true } = {}) {
  if (!settings.enabled) { unreadEvents.value = []; unreadTotal.value = 0; return; }
  const { data, statusCode } = await api.getUnreadBreedingEvents();
  if (statusCode.value !== 200) return;
  unreadEvents.value = data.value?.items || [];
  unreadTotal.value = data.value?.total || 0;
  if (!announce) return;
  const unseen = unseenBreedingEvents(unreadEvents.value, seenEventIDs);
  for (const event of unseen) {
    seenEventIDs.add(event.event_id);
    notifyBrowser(event);
    if (settings.in_app_notifications) message.success(t("breeding.browserBody", { base: event.base_display_name || event.base_name || t("breeding.unknown"), previous: event.previous_count, current: event.current_count }), { duration: 6000 });
  }
  if (unseen.length) persistSeenIDs();
}

async function openNotificationCenter() {
  showNotifications.value = true;
  const { data, statusCode } = await api.getBreedingEvents({ page_size: 200 });
  if (statusCode.value === 200) events.value = data.value?.items || [];
}

async function refreshAll() {
  await Promise.all([loadFarms(), loadCapabilities()]);
  await loadUnreadEvents();
}

async function requestNotificationPermission() {
  if (typeof Notification === "undefined") { message.warning(t("breeding.permissionDenied")); return; }
  const permission = await Notification.requestPermission();
  message[permission === "granted" ? "success" : "warning"](t(permission === "granted" ? "breeding.permissionGranted" : "breeding.permissionDenied"));
}

async function saveSettings() {
  if (settings.selection_mode === "all" && settings.enabled && !confirmAll.value) { message.warning(t("breeding.confirmAll")); return; }
  saving.value = true;
  const { data, statusCode } = await api.updateBreedingNotificationConfig({ ...settings, confirm_all: confirmAll.value });
  saving.value = false;
  if (statusCode.value !== 200) { message.error(errorText(data.value)); return; }
  Object.assign(settings, data.value?.settings || settings);
  showSettings.value = false;
  message.success(t("breeding.saved"));
  await loadUnreadEvents({ announce: false });
}

async function markRead(event) {
  const { statusCode } = await api.markBreedingEventRead(event.event_id);
  if (statusCode.value === 200) { await loadUnreadEvents({ announce: false }); await openNotificationCenter(); }
}

async function markAllRead() {
  const { statusCode } = await api.markAllBreedingEventsRead();
  if (statusCode.value === 200) { await loadUnreadEvents({ announce: false }); await openNotificationCenter(); }
}

onMounted(async () => {
  await loadSettings();
  await refreshAll();
  pollTimer = window.setInterval(async () => { await loadUnreadEvents(); await loadFarms(); }, 20000);
});
onBeforeUnmount(() => window.clearInterval(pollTimer));
</script>

<template>
  <operations-shell title="配种农场" subtitle="查看配种帕鲁、蛋糕余量和可拾取的蛋，并通过游戏内聊天接收可靠的产蛋提醒。" :metadata="metadata" :loading="loading" @refresh="refreshAll">
    <n-alert type="info" :bordered="false" class="section-gap">数据来自实际世界存档。默认每 120 秒检查一次，页面显示的是最近一次成功解析结果，而不是游戏内存实时遥测。</n-alert>
    <n-alert v-if="metadata.is_stale" type="warning" class="section-gap">{{ t("breeding.stale") }} {{ formatDate(metadata.save_file_time || metadata.snapshot_time) }}</n-alert>
    <n-alert v-if="parserStatus.failed" type="error" class="section-gap">{{ t("breeding.lastParseFailed") }} {{ formatDate(parserStatus.failed_at) }}</n-alert>
    <n-alert v-if="!reliable" type="error" class="section-gap">{{ t("breeding.unreliable") }}</n-alert>

    <n-card size="small" class="toolbar-card">
      <div class="toolbar">
        <div>
          <strong>{{ t("breeding.farms") }}</strong>
          <n-tag v-if="capabilities.validated_game_version" size="small" round class="version-tag">{{ t("breeding.validatedVersion", { version: capabilities.validated_game_version }) }}</n-tag>
        </div>
        <n-space>
          <n-badge :value="unreadTotal" :max="99"><n-button secondary @click="openNotificationCenter">{{ t("breeding.notifications") }}</n-button></n-badge>
          <n-button secondary @click="showSettings = true">{{ t("breeding.settings") }}</n-button>
          <n-button type="primary" :loading="loading" @click="refreshAll">{{ t("breeding.refresh") }}</n-button>
        </n-space>
      </div>
      <div class="snapshot-meta">{{ t("breeding.snapshotSaved") }} {{ formatDate(metadata.save_file_time) }}（{{ relativeAge(metadata.save_file_time) }}） · {{ t("breeding.snapshotParsed") }} {{ formatDate(metadata.snapshot_time) }} · {{ t("breeding.syncInterval") }} {{ metadata.sync_interval_seconds || t("breeding.unknown") }}s</div>
      <n-space class="filters">
        <n-select v-model:value="filter.base_id" clearable :placeholder="t('breeding.filterBase')" :options="baseOptions" @update:value="loadFarms" />
        <n-select v-model:value="filter.guild_id" clearable :placeholder="t('breeding.filterGuild')" :options="guildOptions" @update:value="loadFarms" />
        <n-select v-model:value="filter.sort" :options="[{label:t('breeding.sortEgg'),value:'egg_count'},{label:t('breeding.sortCake'),value:'cake_count'},{label:t('breeding.sortLast'),value:'last_egg'}]" @update:value="loadFarms" />
        <n-checkbox v-model:checked="filter.has_egg" @update:checked="loadFarms">{{ t("breeding.withEgg") }}</n-checkbox>
        <n-checkbox v-model:checked="filter.cake_empty" @update:checked="loadFarms">{{ t("breeding.cakeEmpty") }}</n-checkbox>
        <n-checkbox v-model:checked="filter.parent_missing" @update:checked="loadFarms">{{ t("breeding.parentMissing") }}</n-checkbox>
        <n-checkbox v-model:checked="filter.has_warning" @update:checked="loadFarms">{{ t("breeding.warning") }}</n-checkbox>
      </n-space>
    </n-card>

    <n-spin :show="loading">
      <section v-for="group in groupedFarms" :key="group.baseID" class="base-group">
        <header class="base-heading">
          <div><h2>{{ group.baseName }}</h2><span>{{ t("breeding.base") }} ID · {{ shortID(group.baseID) }} · {{ t("breeding.guild") }} · {{ group.guildName }}</span></div>
          <n-space size="small"><n-tag round>{{ group.farms.length }} {{ t("breeding.farms") }}</n-tag><n-tag type="success" round>{{ t("breeding.eggs") }} {{ group.eggTotal }}</n-tag><n-tag round>{{ t("breeding.cake") }} {{ group.cakeTotal }}</n-tag><n-tag v-if="group.abnormal" type="warning" round>{{ t("breeding.abnormal") }} {{ group.abnormal }}</n-tag></n-space>
        </header>
        <div class="farm-grid">
          <n-card v-for="farm in group.farms" :id="`farm-${farm.farm_id}`" :key="farm.farm_id" size="small" class="farm-card" :class="{ highlighted: route.query.farm === farm.farm_id }">
            <template #header><div class="farm-title"><span>{{ t("breeding.farms") }} · {{ shortID(farm.farm_id) }}</span><n-tag :type="statusType(farm.status)" size="small" round>{{ statusLabel(farm.status) }}</n-tag></div></template>
            <div class="metrics">
              <div><span>{{ t("breeding.eggs") }}</span><strong>{{ count(farm.egg_count) }}</strong></div>
              <div><span>{{ t("breeding.cake") }}</span><strong>{{ count(farm.cake_count) }}</strong></div>
              <div><span>{{ t("breeding.lastEgg") }}</span><small>{{ formatDate(farm.last_egg_at) }}</small></div>
            </div>
            <div class="farm-meta"><span>📍 {{ Number(farm.location?.x || 0).toFixed(0) }}, {{ Number(farm.location?.y || 0).toFixed(0) }}, {{ Number(farm.location?.z || 0).toFixed(0) }}</span><n-tag size="small" :type="farm.association_verified && farm.confidence === 'high' ? 'success' : 'warning'">{{ farm.confidence || t("breeding.unknown") }}</n-tag><n-tag size="small" :type="farmMonitored(farm) ? 'success' : 'default'">{{ farmMonitored(farm) ? t("breeding.monitored") : t("breeding.notMonitored") }}</n-tag></div>
            <n-divider />
            <h3>{{ t("breeding.parents") }}</h3>
            <div class="parent-list">
              <div v-for="slot in [0, 1]" :key="slot" class="parent-slot">
                <template v-if="farm.parents?.find(parent => parent.slot_index === slot)">
                  <strong>{{ farm.parents.find(parent => parent.slot_index === slot).nickname || displayPal(farm.parents.find(parent => parent.slot_index === slot).pal_id) }}</strong>
                  <span>{{ displayPal(farm.parents.find(parent => parent.slot_index === slot).pal_id) }} · {{ genderLabel(farm.parents.find(parent => parent.slot_index === slot).gender) }} · Lv.{{ farm.parents.find(parent => parent.slot_index === slot).level }}</span>
                  <span>ID · {{ shortID(farm.parents.find(parent => parent.slot_index === slot).pal_instance_id) }}</span>
                </template>
                <span v-else class="muted">{{ t("breeding.noParent") }}</span>
              </div>
            </div>
            <div v-if="farm.eggs?.length" class="egg-list"><span v-for="egg in farm.eggs" :key="egg.egg_instance_id || egg.egg_item_id">{{ egg.egg_name || t("breeding.unknownEgg") }} × {{ egg.count }}</span></div>
            <n-space v-if="farm.warnings?.length || !farm.parsing_complete" class="warning-list">
              <n-tag v-for="warning in farm.warnings" :key="warning" type="warning" size="small">{{ warning }}</n-tag>
            </n-space>
          </n-card>
        </div>
      </section>
      <n-empty v-if="!loading && !farms.length" :description="t('breeding.noFarms')" class="empty" />
    </n-spin>

    <n-modal v-model:show="showSettings" preset="card" :title="t('breeding.settings')" :style="{ width: isMobile ? 'calc(100vw - 24px)' : '680px' }">
      <n-form label-placement="top">
        <n-form-item :label="t('breeding.enabled')"><n-switch v-model:value="settings.enabled" /></n-form-item>
        <n-form-item :label="t('breeding.selection')"><n-radio-group v-model:value="settings.selection_mode"><n-space><n-radio value="selected">{{ t("breeding.selected") }}</n-radio><n-radio value="all">{{ t("breeding.all") }}</n-radio></n-space></n-radio-group></n-form-item>
        <template v-if="settings.selection_mode === 'selected'">
          <n-form-item :label="t('breeding.selectBases')"><n-select v-model:value="settings.selected_base_ids" multiple clearable :options="baseOptions" /></n-form-item>
          <n-form-item :label="t('breeding.selectFarms')"><n-select v-model:value="settings.selected_farm_ids" multiple clearable :options="farmOptions" /></n-form-item>
        </template>
        <n-alert v-else type="warning" class="section-gap">{{ t("breeding.allWarning") }}<n-checkbox v-model:checked="confirmAll" class="confirm-all">{{ t("breeding.confirmAll") }}</n-checkbox></n-alert>
        <div class="settings-grid">
          <n-form-item :label="t('breeding.minimum')"><n-input-number v-model:value="settings.minimum_ready_eggs" :min="1" :max="10000" /></n-form-item>
          <n-form-item :label="t('breeding.retention')"><n-input-number v-model:value="settings.history_retention_days" :min="1" :max="3650" /></n-form-item>
        </div>
        <n-space vertical>
          <n-checkbox v-model:checked="settings.notify_existing_on_enable">{{ t("breeding.notifyExisting") }}</n-checkbox>
          <n-checkbox v-model:checked="settings.notify_on_each_egg">{{ t("breeding.notifyEach") }}</n-checkbox>
          <n-checkbox v-model:checked="settings.game_notifications">{{ t("breeding.game") }}</n-checkbox>
          <n-checkbox v-model:checked="settings.in_app_notifications">{{ t("breeding.inApp") }}</n-checkbox>
          <n-checkbox v-model:checked="settings.browser_notifications">{{ t("breeding.browser") }}</n-checkbox>
        </n-space>
        <n-form-item v-if="settings.game_notifications" :label="t('breeding.gameMessage')" class="game-message-field">
          <n-input v-model:value="settings.game_notification_message" type="textarea" :maxlength="300" show-count />
          <template #feedback>{{ t("breeding.gameMessageHint", { base: "{base}", new_count: "{new_count}", count: "{count}" }) }}</template>
        </n-form-item>
        <n-button v-if="settings.browser_notifications" secondary class="permission-button" @click="requestNotificationPermission">{{ t("breeding.permission") }}</n-button>
      </n-form>
      <template #footer><n-button type="primary" block :loading="saving" @click="saveSettings">{{ t("breeding.save") }}</n-button></template>
    </n-modal>

    <n-drawer v-model:show="showNotifications" :width="isMobile ? '100%' : 520" placement="right">
      <n-drawer-content :title="t('breeding.notifications')" closable>
        <n-button v-if="unreadTotal" block secondary class="drawer-read-all" @click="markAllRead">{{ t("breeding.markAllRead") }}</n-button>
        <n-list v-if="events.length" bordered>
          <n-list-item v-for="event in events" :key="event.event_id">
            <n-thing :title="t('breeding.browserTitle')" :description="`${event.base_display_name || event.base_name || t('breeding.unknown')} · ${formatDate(event.created_at)}`">
              <template #header-extra><n-tag size="small" :type="event.read ? 'default' : 'success'">{{ event.read ? t("breeding.read") : t("breeding.unread") }}</n-tag></template>
              <p>{{ t("breeding.browserBody", { base: event.base_display_name || event.base_name || t('breeding.unknown'), previous: event.previous_count, current: event.current_count }) }}</p>
              <n-space><n-button size="small" secondary @click="router.push({path:'/breeding-farms',query:{farm:event.farm_id}}); showNotifications=false">{{ t("breeding.viewFarm") }}</n-button><n-button size="small" text @click="markRead(event)">{{ t("breeding.markRead") }}</n-button></n-space>
            </n-thing>
          </n-list-item>
        </n-list>
        <n-empty v-else :description="t('breeding.noEvents')" />
      </n-drawer-content>
    </n-drawer>
  </operations-shell>
</template>

<style scoped>
.drawer-read-all { margin-bottom: 12px; }
.section-gap{margin-bottom:14px}.toolbar-card{margin-bottom:22px}.toolbar{display:flex;align-items:center;justify-content:space-between;gap:16px}.toolbar>div:first-child{display:flex;align-items:center;gap:10px}.version-tag{max-width:min(46vw,460px);overflow:hidden}.snapshot-meta{margin-top:10px;font-size:12px;opacity:.55}.filters{margin-top:16px;align-items:center}.filters .n-select{width:180px}.base-group{margin:26px 0}.base-heading{display:flex;align-items:end;justify-content:space-between;gap:16px;margin-bottom:12px}.base-heading h2{margin:0;font-size:21px}.base-heading span{font-size:13px;opacity:.58}.farm-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(330px,1fr));gap:14px}.farm-card{transition:border-color .2s,box-shadow .2s}.farm-card.highlighted{border-color:#18a058;box-shadow:0 0 0 3px rgba(24,160,88,.12)}.farm-title{display:flex;align-items:center;justify-content:space-between;gap:12px}.metrics{display:grid;grid-template-columns:.8fr .8fr 1.5fr;gap:10px}.metrics>div{padding:10px;border-radius:9px;background:rgba(128,128,128,.07)}.metrics span,.metrics small{display:block;font-size:12px;opacity:.58}.metrics strong{display:block;margin-top:4px;font-size:25px}.metrics small{margin-top:7px;line-height:1.35}.farm-meta{display:flex;gap:7px;align-items:center;margin-top:10px;font-size:12px;opacity:.8}.farm-meta>span{margin-right:auto}.egg-list{display:flex;flex-wrap:wrap;gap:7px;margin-top:10px}.egg-list span{padding:5px 8px;border-radius:7px;background:rgba(24,160,88,.09);font-size:12px}h3{margin:0 0 10px;font-size:14px}.parent-list{display:grid;grid-template-columns:1fr 1fr;gap:8px}.parent-slot{min-height:46px;padding:9px;border:1px solid rgba(128,128,128,.16);border-radius:8px}.parent-slot strong,.parent-slot span{display:block}.parent-slot span{font-size:12px;opacity:.62;margin-top:3px}.muted{opacity:.45!important}.warning-list{margin-top:12px}.empty{padding:54px}.settings-grid{display:grid;grid-template-columns:1fr 1fr;gap:16px}.confirm-all{display:block;margin-top:10px}.game-message-field{margin-top:16px}.permission-button{margin-top:16px}
@media(max-width:767px){.toolbar{align-items:flex-start;flex-direction:column}.toolbar>.n-space{width:100%;display:grid!important;grid-template-columns:1fr 1fr}.toolbar>.n-space>:last-child{grid-column:1/-1}.toolbar .n-button{width:100%}.filters{display:grid!important;grid-template-columns:1fr 1fr;width:100%}.filters .n-select{width:100%}.filters .n-select:nth-child(3){grid-column:1/-1}.base-heading{align-items:flex-start;flex-direction:column}.farm-grid{grid-template-columns:1fr}.metrics{grid-template-columns:1fr 1fr}.metrics>div:last-child{grid-column:1/-1}.farm-meta{flex-wrap:wrap}.farm-meta>span{width:100%}.settings-grid{grid-template-columns:1fr}}
</style>
