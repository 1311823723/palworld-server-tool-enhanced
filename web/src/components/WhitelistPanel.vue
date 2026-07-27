<script setup>
import { computed, onMounted, ref } from "vue";
import ApiService from "@/service/api";
import { useDialog, useMessage } from "naive-ui";

const api = new ApiService();
const message = useMessage();
const dialog = useDialog();
const loading = ref(false);
const saving = ref(false);
const query = ref("");
const players = ref([]);
const whitelist = ref([]);
const selectedUID = ref("");

const playerOptions = computed(() => players.value
  .filter((player) => !whitelist.value.some((item) => item.player_uid === player.player_uid))
  .map((player) => ({ label: `${player.nickname || "未命名玩家"} · ${player.player_uid}`, value: player.player_uid })));
const visible = computed(() => {
  const keyword = query.value.trim().toLocaleLowerCase("zh-CN");
  if (!keyword) return whitelist.value;
  return whitelist.value.filter((item) => `${item.name} ${item.player_uid} ${item.steam_id}`.toLocaleLowerCase("zh-CN").includes(keyword));
});

async function load() {
  loading.value = true;
  const [whiteResponse, playerResponse] = await Promise.all([
    api.getWhitelist(),
    api.getPlayerList({ order_by: "last_online", desc: true }),
  ]);
  loading.value = false;
  if (whiteResponse.statusCode.value !== 200) {
    message.error(whiteResponse.data.value?.error || "白名单读取失败");
    return;
  }
  whitelist.value = Array.isArray(whiteResponse.data.value) ? whiteResponse.data.value : [];
  players.value = Array.isArray(playerResponse.data.value) ? playerResponse.data.value : [];
}

async function add() {
  const player = players.value.find((item) => item.player_uid === selectedUID.value);
  if (!player || saving.value) return;
  saving.value = true;
  const { data, statusCode } = await api.addWhitelist({
    name: player.nickname,
    steam_id: player.steam_id,
    player_uid: player.player_uid,
  });
  saving.value = false;
  if (statusCode.value !== 200) message.error(data.value?.error || "添加白名单失败");
  else {
    selectedUID.value = "";
    message.success("玩家已加入白名单");
    await load();
  }
}

function remove(item) {
  dialog.warning({
    title: "移出白名单",
    content: `确定将“${item.name || item.player_uid}”移出白名单吗？`,
    positiveText: "移出",
    negativeText: "取消",
    onPositiveClick: async () => {
      const { data, statusCode } = await api.removeWhitelist(item);
      if (statusCode.value !== 200) message.error(data.value?.error || "移除失败");
      else await load();
    },
  });
}

onMounted(load);
</script>

<template>
  <section class="whitelist-panel">
    <header>
      <div><span>访问控制</span><h2>服务器白名单</h2><p>白名单检查开启后，未列入的玩家会被自动移出服务器。</p></div>
      <n-button secondary :loading="loading" @click="load">刷新</n-button>
    </header>
    <div class="add-row">
      <n-select v-model:value="selectedUID" filterable clearable :options="playerOptions" placeholder="从已有玩家中选择" />
      <n-button type="primary" :disabled="!selectedUID" :loading="saving" @click="add">加入白名单</n-button>
      <n-input v-model:value="query" clearable placeholder="搜索白名单" />
    </div>
    <div v-if="visible.length" class="white-list">
      <article v-for="item in visible" :key="`${item.player_uid}-${item.steam_id}`">
        <span class="avatar">{{ (item.name || "?").slice(0, 1) }}</span>
        <div><strong>{{ item.name || "未命名玩家" }}</strong><small>UID {{ item.player_uid || "—" }} · Steam {{ item.steam_id || "—" }}</small></div>
        <n-button size="small" type="error" text @click="remove(item)">移出</n-button>
      </article>
    </div>
    <n-empty v-else-if="!loading" description="白名单目前为空" class="empty" />
  </section>
</template>

<style scoped>
.whitelist-panel { padding: 18px; border: 1px solid var(--ops-line); background: var(--ops-panel); }
header { display: flex; justify-content: space-between; gap: 18px; }
header span { color: var(--ops-accent); font-size: 11px; font-weight: 700; letter-spacing: .08em; }
h2 { margin: 5px 0; font-size: 22px; }
p { color: var(--ops-muted); font-size: 13px; }
.add-row { display: grid; grid-template-columns: minmax(240px, 1fr) auto minmax(180px, 280px); gap: 9px; margin: 18px 0 10px; }
.white-list { display: grid; }
.white-list article { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 11px; min-height: 64px; border-top: 1px solid var(--ops-line); }
.avatar { display: grid; place-items: center; width: 36px; height: 36px; border-radius: 9px; color: var(--ops-accent); background: var(--ops-accent-soft); font-weight: 700; }
.white-list strong, .white-list small { display: block; }
.white-list small { margin-top: 3px; overflow: hidden; color: var(--ops-muted); font: 11px/1.4 ui-monospace, monospace; text-overflow: ellipsis; white-space: nowrap; }
.empty { padding: 60px 0; }
@media (max-width: 680px) { .add-row { grid-template-columns: 1fr auto; } .add-row > :last-child { grid-column: 1 / -1; } }
</style>
