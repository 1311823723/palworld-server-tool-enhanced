<script setup>
import { computed, h, onMounted, ref, watch } from "vue";
import { NButton, NTag, useMessage } from "naive-ui";
import ApiService from "@/service/api";
import OperationsShell from "@/components/OperationsShell.vue";
import pageStore from "@/stores/model/page";
import itemsMap from "@/assets/items.json";
import { loadOnce } from "@/utils/enhancedViews";

const api = new ApiService();
const message = useMessage();
const loading = ref(false);
const items = ref([]);
const metadata = ref({ warnings: [] });
const total = ref(0);
const page = ref(1);
const pageSize = ref(50);
const search = ref("");
const sourceType = ref("");
const sort = ref("count_desc");
const showLocations = ref(false);
const selectedItem = ref(null);
const locations = ref([]);
const locationsLoading = ref(false);
const isMobile = computed(() => pageStore().getScreenWidth() < 768);
let timer;
const locationCache = new Map();

const localeItems = computed(() => itemsMap.zh || []);
const itemByID = computed(() => Object.fromEntries(localeItems.value.flatMap((item) => [[item.key.toLowerCase(), item], [item.id.toLowerCase(), item]])));
const labelFor = (id, fallback = "", displayName = "") => displayName || itemByID.value[id?.toLowerCase()]?.name || fallback || id;
const iconModules = import.meta.glob("/src/assets/items/*.webp", { eager: true, query: "?url", import: "default" });
const iconFor = (id) => iconModules[`/src/assets/items/${String(id || "").toLowerCase()}.webp`] || "";
const sourceLabel = (source) => ({ player_common: "玩家背包", player_drop: "玩家掉落栏", player_essential: "玩家关键物品", player_food: "玩家食物栏", player_equipment: "玩家装备", base_container: "据点容器", base_storage: "据点容器", base_feed_box: "据点饲料箱", breeding_farm_cake_box: "配种农场蛋糕箱" }[source] || source || "未知来源");
const errorText = (data) => data?.error || "库存请求失败";

async function loadSummary() {
  loading.value = true;
  const { data, statusCode } = await api.getInventorySummary({ page: page.value, page_size: pageSize.value, q: search.value, source_type: sourceType.value, sort: sort.value });
  loading.value = false;
  if (statusCode.value !== 200) { message.error(errorText(data.value)); return; }
  items.value = data.value?.items || [];
  metadata.value = data.value?.metadata || metadata.value;
  total.value = data.value?.total || 0;
}

async function openLocations(item) {
  selectedItem.value = item;
  showLocations.value = true;
  locationsLoading.value = true;
  try {
    locations.value = await loadOnce(locationCache, `${item.item_id}:${sourceType.value}`, async () => {
      const { data, statusCode } = await api.getInventoryItemLocations(item.item_id, { page_size: 200, source_type: sourceType.value });
      if (statusCode.value !== 200) throw new Error(errorText(data.value));
      return data.value?.items || [];
    });
  } catch (error) { message.error(error.message); }
  finally { locationsLoading.value = false; }
}

const columns = computed(() => [
  { title: "物品", key: "item_id", render: (row) => h("div", { class: "item-cell", style: "display:flex;align-items:center;gap:12px" }, [iconFor(row.item_id) ? h("img", { src: iconFor(row.item_id), alt: "", class: "item-icon", style: "width:42px;height:42px;object-fit:contain;border-radius:8px" }) : null, h("div", [h("strong", labelFor(row.item_id, row.item_name, row.item_display_name)), h("small", { style: "display:block;opacity:.5;margin-top:3px" }, row.item_id)])]) },
  { title: "总数量", key: "total_count", sorter: "default", render: (row) => Number(row.total_count).toLocaleString() },
  { title: "玩家 / 据点", key: "split", render: (row) => `${Number(row.player_total).toLocaleString()} / ${Number(row.base_total).toLocaleString()}` },
  { title: "位置", key: "locations", render: (row) => h(NTag, { size: "small", round: true }, { default: () => `${row.location_count} 格 · ${row.container_count} 容器` }) },
  { title: "", key: "actions", render: (row) => h(NButton, { size: "small", secondary: true, onClick: () => openLocations(row) }, { default: () => "查看位置" }) },
]);

watch([sourceType, sort, page, pageSize], loadSummary);
watch(search, () => { clearTimeout(timer); timer = setTimeout(() => { page.value = 1; loadSummary(); }, 300); });
onMounted(loadSummary);
</script>

<template>
  <operations-shell title="全服库存" subtitle="按物品查看数量，以及它们所在的背包和据点容器。" :metadata="metadata" :loading="loading" @refresh="loadSummary">
    <n-alert v-if="metadata.is_stale" type="warning" class="mb-4">存档快照已过期：{{ metadata.save_file_time || metadata.snapshot_time }}</n-alert>
    <n-card size="small">
      <template #header>库存总览 <n-tag size="small" round>{{ total }} 种物品</n-tag></template>
      <template #header-extra><n-button size="small" :loading="loading" @click="loadSummary">刷新</n-button></template>
      <n-space class="filters mb-4">
        <n-input v-model:value="search" clearable placeholder="搜索物品 ID 或名称" style="min-width:260px" />
        <n-select v-model:value="sourceType" clearable placeholder="全部来源" :options="[{label:'玩家背包',value:'player_common'},{label:'玩家食物栏',value:'player_food'},{label:'玩家装备',value:'player_equipment'},{label:'据点容器',value:'base_storage'},{label:'据点饲料箱',value:'base_feed_box'},{label:'配种农场蛋糕箱',value:'breeding_farm_cake_box'}]" style="width:190px" />
        <n-select v-model:value="sort" :options="[{label:'数量从高到低',value:'count_desc'},{label:'数量从低到高',value:'count_asc'},{label:'名称',value:'name'},{label:'位置数量',value:'locations'}]" style="width:170px" />
      </n-space>
      <n-spin :show="loading">
        <n-data-table v-if="!isMobile" :columns="columns" :data="items" :row-key="row => row.item_id" :bordered="false" />
        <div v-else class="inventory-cards">
          <n-card v-for="item in items" :key="item.item_id" size="small">
            <div class="item-cell">
              <img v-if="iconFor(item.item_id)" :src="iconFor(item.item_id)" alt="" class="item-icon" />
              <div><strong>{{ labelFor(item.item_id, item.item_name, item.item_display_name) }}</strong><small>{{ item.item_id }}</small></div>
            </div>
            <div class="count-row"><strong>{{ Number(item.total_count).toLocaleString() }}</strong><span>玩家 {{ item.player_total }} · 据点 {{ item.base_total }}</span></div>
            <n-button block secondary @click="openLocations(item)">查看 {{ item.location_count }} 个位置</n-button>
          </n-card>
        </div>
      </n-spin>
      <n-empty v-if="!loading && !items.length" description="没有符合筛选条件的库存记录" />
      <n-pagination v-if="total > pageSize" v-model:page="page" v-model:page-size="pageSize" :item-count="total" :page-sizes="[20,50,100,200]" show-size-picker class="mt-4" />
    </n-card>

    <n-drawer v-model:show="showLocations" :width="isMobile ? '100%' : 620" placement="right">
      <n-drawer-content :title="selectedItem ? `${labelFor(selectedItem.item_id, selectedItem.item_name, selectedItem.item_display_name)} 的位置` : '库存位置'" closable>
        <n-spin :show="locationsLoading">
          <n-list bordered>
            <n-list-item v-for="location in locations" :key="location.location_id">
              <n-thing :title="location.container_name || sourceLabel(location.source_type)" :description="`${sourceLabel(location.source_type)} · 槽位 ${location.slot_index}`">
                <template #header-extra><strong>× {{ location.count }}</strong></template>
                <n-space size="small"><n-tag v-if="location.player_name" size="small">玩家：{{ location.player_name }}</n-tag><n-tag v-if="location.guild_name" size="small">公会：{{ location.guild_name }}</n-tag><n-tag v-if="location.base_display_name" size="small">据点：{{ location.base_display_name }}</n-tag></n-space>
              </n-thing>
            </n-list-item>
          </n-list>
          <n-empty v-if="!locationsLoading && !locations.length" description="该物品没有可用位置" />
        </n-spin>
      </n-drawer-content>
    </n-drawer>
  </operations-shell>
</template>

<style scoped>
.item-cell{display:flex;align-items:center;gap:12px}.item-cell small{display:block;opacity:.5;margin-top:3px}.item-icon{width:42px;height:42px;object-fit:contain;border-radius:8px;background:rgba(128,128,128,.08)}
.inventory-cards{display:grid;gap:12px}.count-row{display:flex;justify-content:space-between;align-items:baseline;margin:16px 0}.count-row strong{font-size:24px}.count-row span{opacity:.65}.mb-4{margin-bottom:16px}.mt-4{margin-top:16px}
@media(max-width:767px){.filters{display:grid!important}.filters>*{width:100%!important}}
</style>
