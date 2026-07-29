<script setup>
import { computed, h, onMounted, ref, watch } from "vue";
import { NProgress, NTag, useMessage } from "naive-ui";
import ApiService from "@/service/api";
import OperationsShell from "@/components/OperationsShell.vue";
import pageStore from "@/stores/model/page";
import palMap from "@/assets/pal.json";
import itemsMap from "@/assets/items.json";
import { isWorkerAbnormal } from "@/utils/enhancedViews";
import { localizedPalName } from "@/utils/gameLabels";

const api = new ApiService();
const message = useMessage();
const loading = ref(false);
const detailLoading = ref(false);
const bases = ref([]);
const selectedBase = ref("");
const workers = ref([]);
const feeds = ref([]);
const metadata = ref({ warnings: [], capabilities: {} });
const total = ref(0);
const page = ref(1);
const pageSize = ref(50);
const search = ref("");
const abnormalOnly = ref(false);
const sort = ref("name");
const isMobile = computed(() => pageStore().getScreenWidth() < 768);
let searchTimer;

const displayPal = (id) => localizedPalName(id, palMap, "zh");
const itemIndex = computed(() => Object.fromEntries((itemsMap.zh || []).map((item) => [item.key.toLowerCase(), item.name])));
const displayItem = (id) => itemIndex.value[id?.toLowerCase()] || id || "—";
const percent = (value, maximum) => value == null || !maximum ? null : Math.max(0, Math.min(100, value * 100 / maximum));
const statusType = (worker) => isAbnormal(worker) ? "error" : "success";
const isAbnormal = isWorkerAbnormal;
const errorText = (data) => data?.error || "请求失败，请检查存档同步状态";

const columns = computed(() => [
  { title: "帕鲁", key: "pal_id", render: (row) => h("div", [h("strong", row.nickname || displayPal(row.pal_id)), h("small", { style: "display:block;opacity:.6" }, `${displayPal(row.pal_id)} · Lv.${row.level}`)]) },
  { title: "生命", key: "hp", width: 150, render: (row) => row.hp == null ? "—" : h(NProgress, { percentage: Math.round(percent(row.hp, row.max_hp) || 0), status: percent(row.hp, row.max_hp) < 30 ? "error" : "success", height: 8, indicatorPlacement: "inside" }) },
  { title: "饱食 / SAN", key: "condition", render: (row) => `${row.full_stomach == null ? "—" : Math.round(row.full_stomach)} / ${row.sanity == null ? "—" : Math.round(row.sanity)}` },
  { title: "当前工作", key: "current_work", render: (row) => row.current_work || "—" },
  { title: "工作速度", key: "work_speed", sorter: "default" },
  { title: "状态", key: "status", render: (row) => h(NTag, { type: statusType(row), size: "small", round: true }, { default: () => isAbnormal(row) ? "需关注" : "正常" }) },
]);

async function loadBases() {
  loading.value = true;
  const { data, statusCode } = await api.getBaseCamps();
  loading.value = false;
  if (statusCode.value !== 200) { message.error(errorText(data.value)); return; }
  bases.value = data.value?.items || [];
  metadata.value = data.value?.metadata || metadata.value;
  if (!selectedBase.value && bases.value.length) selectedBase.value = bases.value[0].base_id;
}

async function loadDetails() {
  if (!selectedBase.value) return;
  detailLoading.value = true;
  const params = { page: page.value, page_size: pageSize.value, q: search.value, sort: sort.value, abnormal_only: abnormalOnly.value || undefined };
  const [workersResponse, feedsResponse] = await Promise.all([api.getBaseWorkPals(selectedBase.value, params), api.getBaseFeedBoxes(selectedBase.value)]);
  detailLoading.value = false;
  if (workersResponse.statusCode.value !== 200) { message.error(errorText(workersResponse.data.value)); return; }
  workers.value = workersResponse.data.value?.items || [];
  total.value = workersResponse.data.value?.total || 0;
  metadata.value = workersResponse.data.value?.metadata || metadata.value;
  feeds.value = feedsResponse.statusCode.value === 200 ? feedsResponse.data.value?.items || [] : [];
}

watch(selectedBase, () => { page.value = 1; loadDetails(); });
watch([abnormalOnly, sort, page, pageSize], loadDetails);
watch(search, () => { clearTimeout(searchTimer); searchTimer = setTimeout(() => { page.value = 1; loadDetails(); }, 300); });
onMounted(loadBases);
</script>

<template>
  <operations-shell title="工作帕鲁" subtitle="按据点查看工作状态、饱食度、SAN 和饲料储备。" :metadata="metadata" :loading="loading || detailLoading" @refresh="loadBases">
    <n-alert v-if="metadata.is_stale" type="warning" class="mb-4">快照可能已过期，显示时间：{{ metadata.save_file_time || metadata.snapshot_time }}</n-alert>
    <n-alert v-for="warning in metadata.warnings || []" :key="warning" type="info" class="mb-2">{{ warning }}</n-alert>
    <n-grid :cols="isMobile ? 1 : 4" :x-gap="16" :y-gap="16">
      <n-gi>
        <n-card title="据点" size="small">
          <n-skeleton v-if="loading" text :repeat="5" />
          <n-empty v-else-if="!bases.length" description="尚无据点快照；请先运行一次存档同步" />
          <n-radio-group v-else v-model:value="selectedBase" class="base-list">
            <n-radio-button v-for="base in bases" :key="base.base_id" :value="base.base_id">
              <span>{{ base.display_name }}</span>
              <small>{{ base.worker_pal_count }}/{{ base.max_worker_pals }} · 异常 {{ base.hungry_pal_count + base.low_sanity_pal_count + base.sick_pal_count + base.down_pal_count }}</small>
            </n-radio-button>
          </n-radio-group>
        </n-card>
      </n-gi>
      <n-gi :span="isMobile ? 1 : 3">
        <n-card size="small">
          <template #header>工作帕鲁 <n-tag size="small" round>{{ total }}</n-tag></template>
          <template #header-extra><n-button size="small" :loading="detailLoading" @click="loadDetails">刷新</n-button></template>
          <n-space class="mb-4" align="center">
            <n-input v-model:value="search" clearable placeholder="搜索帕鲁、昵称、主人或工作" style="min-width:240px" />
            <n-select v-model:value="sort" :options="[{label:'名称',value:'name'},{label:'等级',value:'level'},{label:'低生命优先',value:'hp_percent'},{label:'低饱食优先',value:'full_stomach'},{label:'低 SAN 优先',value:'sanity'},{label:'工作速度',value:'work_speed'}]" style="width:150px" />
            <n-checkbox v-model:checked="abnormalOnly">只看异常</n-checkbox>
          </n-space>
          <n-spin :show="detailLoading">
            <n-data-table v-if="!isMobile" :columns="columns" :data="workers" :row-key="row => row.instance_id" :bordered="false" />
            <div v-else class="worker-cards">
              <n-card v-for="worker in workers" :key="worker.instance_id" size="small">
                <template #header>{{ worker.nickname || displayPal(worker.pal_id) }} <small>Lv.{{ worker.level }}</small></template>
                <template #header-extra><n-tag :type="statusType(worker)" size="small">{{ isAbnormal(worker) ? "需关注" : "正常" }}</n-tag></template>
                <n-descriptions label-placement="left" :column="1" size="small">
                  <n-descriptions-item label="种类">{{ displayPal(worker.pal_id) }}</n-descriptions-item>
                  <n-descriptions-item label="生命">{{ worker.hp == null ? "—" : `${worker.hp}/${worker.max_hp}` }}</n-descriptions-item>
                  <n-descriptions-item label="饱食 / SAN">{{ worker.full_stomach == null ? "—" : Math.round(worker.full_stomach) }} / {{ worker.sanity == null ? "—" : Math.round(worker.sanity) }}</n-descriptions-item>
                  <n-descriptions-item label="工作">{{ worker.current_work || "—" }}</n-descriptions-item>
                  <n-descriptions-item label="工作速度">{{ worker.work_speed || "—" }}</n-descriptions-item>
                </n-descriptions>
              </n-card>
            </div>
          </n-spin>
          <n-pagination v-if="total > pageSize" v-model:page="page" v-model:page-size="pageSize" :item-count="total" :page-sizes="[20,50,100]" show-size-picker class="mt-4" />
        </n-card>
        <n-card title="饲料箱" size="small" class="mt-4">
          <n-empty v-if="!feeds.length" description="当前快照没有可识别的饲料箱数据" />
          <n-grid v-else :cols="isMobile ? 1 : 3" :x-gap="12" :y-gap="12">
            <n-gi v-for="box in feeds" :key="box.container_id"><n-card size="small" :title="box.display_name || '饲料箱'">
              <div class="feed-list"><div v-for="slot in box.slots" :key="slot.location_id" class="feed-row"><span>{{ displayItem(slot.item_id) }}</span><strong>× {{ slot.count }}</strong></div></div>
              <n-text depth="3">合计 {{ box.total_item_count }}</n-text>
            </n-card></n-gi>
          </n-grid>
        </n-card>
      </n-gi>
    </n-grid>
  </operations-shell>
</template>

<style scoped>
.base-list { display:flex; flex-direction:column; gap:8px; width:100%; }
.base-list :deep(.n-radio-button) { width:100%; border-radius:8px!important; }
.base-list :deep(.n-radio-button__state-border) { border-radius:8px!important; }
.base-list span, .base-list small { display:block; text-align:left; }
.base-list small { opacity:.58; margin-top:3px; }
.worker-cards { display:grid; gap:12px; }
.feed-list{display:grid;gap:10px;margin-bottom:12px}.feed-row{display:flex;align-items:center;justify-content:space-between;gap:12px}.feed-row strong{white-space:nowrap}
.mb-2{margin-bottom:8px}.mb-4{margin-bottom:16px}.mt-4{margin-top:16px}
@media(max-width:767px){.base-list{flex-direction:row;overflow-x:auto}.base-list :deep(.n-radio-button){min-width:210px}.mb-4{flex-wrap:wrap!important}}
</style>
