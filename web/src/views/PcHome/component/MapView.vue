<script setup>
import { useI18n } from "vue-i18n";
import "leaflet/dist/leaflet.css";
import {
  LCircle,
  LIcon,
  LMap,
  LMarker,
  LPopup,
  LTileLayer,
  LTooltip,
} from "@vue-leaflet/vue-leaflet";
import { AddCircle20Filled, SubtractCircle20Filled } from "@vicons/fluent";
import { ChevronDown, ChevronUp } from "@vicons/ionicons5";
import { useDialog, useMessage } from "naive-ui";
import ApiService from "@/service/api.js";
import IconBase from "@/assets/map/base.webp";
import IconPlayer from "@/assets/map/player.webp";
import IconBossTower from "@/assets/map/boss_tower.webp";
import IconFastTravel from "@/assets/map/fast_travel.webp";
import playerToGuildStore from "@/stores/model/playerToGuild.js";
import points from "@/assets/map/points.json";
import {
  buildPlayerGuildMap,
  mergeMapPlayers,
  selectVisibleMapPlayers,
} from "@/utils/mapPlayers.js";
import { baseDisplayName, validateBaseAliasName } from "@/utils/baseAliases.js";

const { t } = useI18n();

const LAND_SCAPE = [349400, 724400, -1099400, -724400];

const api = new ApiService();
const message = useMessage();
const dialog = useDialog();

const mousePosition = ref([0, 0]);
const zoom = ref(2);
const tiles = ref("map/tiles/{z}/{x}/{y}.png");
const playerList = ref([]);
const guildList = ref([]);
const onlinePlayerIds = ref(new Set());
const showPlayer = ref(true);
const playerVisibility = ref("online");
const showBaseCamp = ref(true);
const showBossTower = ref(false);
const showFastTravel = ref(false);
const mapRef = ref(null);
const searchTarget = ref(null);
const controlCollapsed = ref(false);
const editingBaseID = ref("");
const aliasName = ref("");
const aliasSaving = ref(false);
const isAdmin = computed(() => Boolean(localStorage.getItem("palworld_token")));

let timer = null;
let stopped = false;

const toMapPosition = (position) => {
  // hack
  if (position[0] >= -256 && position[0] <= 256) {
    return position;
  }
  const x =
    -256 +
    (256 * (position[0] - LAND_SCAPE[2])) / (LAND_SCAPE[0] - LAND_SCAPE[2]);
  const y =
    (256 * (position[1] - LAND_SCAPE[3])) / (LAND_SCAPE[1] - LAND_SCAPE[3]);
  return [x, y];
};

const fromMapPosition = (mapPosition) => {
  // 还原 x 坐标
  const worldX =
    ((mapPosition[0] + 256) * (LAND_SCAPE[0] - LAND_SCAPE[2])) / 256 +
    LAND_SCAPE[2];
  // 还原 y 坐标
  const worldY =
    (mapPosition[1] * (LAND_SCAPE[1] - LAND_SCAPE[3])) / 256 + LAND_SCAPE[3];

  // 保留两位小数
  return [worldX.toFixed(2), worldY.toFixed(2)];
};

const toMapDistance = (distance) => {
  return 256 * (distance / (LAND_SCAPE[0] - LAND_SCAPE[2]));
};

const rawBaseMarkers = computed(() =>
  guildList.value.flatMap((guild, guildIndex) =>
    (guild.base_camp || []).map((camp, campIndex) => ({
      key: `${guildIndex}-${campIndex}`,
      guild,
      camp,
      campIndex,
      displayName: baseDisplayName(camp, campIndex),
      position: toMapPosition([camp.location_x, camp.location_y]),
    })),
  ),
);

const playerGuilds = computed(() => buildPlayerGuildMap(guildList.value));
const visiblePlayerList = computed(() =>
  selectVisibleMapPlayers(
    playerList.value,
    onlinePlayerIds.value,
    playerVisibility.value,
  ),
);
const playerVisibilityOptions = computed(() => [
  { label: t("map.onlinePlayers"), value: "online" },
  { label: t("filter.allPlayers"), value: "all" },
]);
const isPlayerOnline = (player) => onlinePlayerIds.value.has(player.player_uid);
const playerGuild = (player) => playerGuilds.value.get(player.player_uid);
const guildDisplayName = (guild) => guild?.name || t("filter.unnamedGuild");
const playerInitial = (player) =>
  (player.nickname || player.player_uid || "?").trim().charAt(0).toUpperCase();

const clusteredBaseMarkers = computed(() => {
  if (zoom.value >= 5) {
    return rawBaseMarkers.value.map((marker) => ({
      key: marker.key,
      position: marker.position,
      markers: [marker],
    }));
  }

  const cellSize = 48 / 2 ** zoom.value;
  const clusters = new Map();
  rawBaseMarkers.value.forEach((marker) => {
    const clusterKey = `${Math.floor(marker.position[0] / cellSize)}:${Math.floor(
      marker.position[1] / cellSize,
    )}`;
    if (!clusters.has(clusterKey)) clusters.set(clusterKey, []);
    clusters.get(clusterKey).push(marker);
  });

  return Array.from(clusters.entries()).map(([key, markers]) => ({
    key,
    markers,
    position: [
      markers.reduce((sum, marker) => sum + marker.position[0], 0) /
        markers.length,
      markers.reduce((sum, marker) => sum + marker.position[1], 0) /
        markers.length,
    ],
  }));
});

const searchOptions = computed(() => [
  ...visiblePlayerList.value.map((player) => ({
    label: `${t(isPlayerOnline(player) ? "status.online" : "status.offline")} · ${player.nickname || player.player_uid}`,
    value: `player:${player.player_uid}`,
  })),
  ...rawBaseMarkers.value.map((marker) => ({
    label: `${marker.displayName} · ${marker.guild.name || t("filter.unnamedGuild")}`,
    value: `base:${marker.key}`,
  })),
]);

const allGuildBases = computed(() => guildList.value.flatMap((guild) => guild.base_camp || []));
const aliasError = computed(() => validateBaseAliasName(aliasName.value, editingBaseID.value, allGuildBases.value));
const clusterLabel = (cluster) => cluster.markers.length > 1
  ? t("map.clusterTitle", { count: cluster.markers.length })
  : cluster.markers[0].displayName;

const refreshGuilds = async () => {
  const { data, statusCode } = await api.getGuildList();
  if (statusCode.value === 200 && Array.isArray(data.value)) guildList.value = data.value;
};

const beginRenameBase = (marker) => {
  editingBaseID.value = marker.camp.id || "";
  aliasName.value = marker.camp.custom_name || marker.displayName;
};

const cancelRenameBase = () => {
  editingBaseID.value = "";
  aliasName.value = "";
};

const saveBaseAlias = async () => {
  if (aliasSaving.value || aliasError.value || !editingBaseID.value) return;
  aliasSaving.value = true;
  const { data, statusCode } = await api.updateBaseAlias(editingBaseID.value, { name: aliasName.value.trim() });
  aliasSaving.value = false;
  if (statusCode.value !== 200) {
    message.error(data.value?.error || "据点名称保存失败");
    return;
  }
  cancelRenameBase();
  await refreshGuilds();
  message.success("据点名称已更新");
};

const resetBaseAlias = (marker) => {
  dialog.warning({
    title: "重置据点名称",
    content: `确定把“${marker.displayName}”恢复为“据点 ${marker.campIndex + 1}”吗？`,
    positiveText: "重置",
    negativeText: "取消",
    onPositiveClick: async () => {
      const { data, statusCode } = await api.deleteBaseAlias(marker.camp.id);
      if (statusCode.value !== 200) {
        message.error(data.value?.error || "据点名称重置失败");
        return;
      }
      cancelRenameBase();
      await refreshGuilds();
      message.success("已恢复默认据点名称");
    },
  });
};

const focusSearchTarget = (value) => {
  if (!value) return;
  const [type, id] = value.split(":");
  let position;
  if (type === "player") {
    const player = playerList.value.find((item) => item.player_uid === id);
    if (player)
      position = toMapPosition([player.location_x, player.location_y]);
  } else {
    position = rawBaseMarkers.value.find(
      (marker) => marker.key === id,
    )?.position;
  }
  if (position) {
    zoom.value = 5;
    mapRef.value?.leafletObject?.setView(position, 5);
  }
};

const setMarkerAccessibility = (marker, label) => {
  const element = marker?.getElement?.();
  if (!element) return;
  element.setAttribute("aria-label", label);
  element.setAttribute("alt", label);
};

const toPlayer = (uid) => {
  playerToGuildStore().setCurrentUid(uid);
  playerToGuildStore().setUpdateStatus("players");
};

const toGuild = (guild) => {
  if (!guild?.admin_player_uid) return;
  playerToGuildStore().setCurrentUid(guild.admin_player_uid);
  playerToGuildStore().setUpdateStatus("guilds");
};

const refreshPlayer = async () => {
  try {
    const { data } = await api.getOnlinePlayerList();
    if (Array.isArray(data.value)) {
      const onlinePlayers = data.value;
      onlinePlayerIds.value = new Set(
        onlinePlayers.map((player) => player.player_uid).filter(Boolean),
      );
      playerList.value = mergeMapPlayers(playerList.value, onlinePlayers);
    }
  } finally {
    if (!stopped) timer = setTimeout(refreshPlayer, 5000);
  }
};

const onMapMouseMove = (event) => {
  mousePosition.value = [
    event.latlng.lat.toFixed(2),
    event.latlng.lng.toFixed(2),
  ];
};

// 左下角控件
const onAddZoom = () => {
  if (zoom.value !== 6) {
    zoom.value += 1;
  }
};
const onSubtractZoom = () => {
  if (zoom.value !== 0) {
    zoom.value -= 1;
  }
};

onMounted(async () => {
  const [playerResponse, guildResponse] = await Promise.all([
    api.getPlayerList({}),
    api.getGuildList(),
  ]);
  playerList.value = Array.isArray(playerResponse.data.value)
    ? playerResponse.data.value
    : [];
  guildList.value = Array.isArray(guildResponse.data.value)
    ? guildResponse.data.value
    : [];

  await refreshPlayer();
});

onUnmounted(() => {
  stopped = true;
  clearTimeout(timer);
});
</script>

<template>
  <div class="map-view h-full">
    <l-map
      ref="mapRef"
      style="width: 100%; height: 100%"
      crs="Simple"
      v-model:zoom="zoom"
      :use-global-leaflet="false"
      :center="[-128, 128]"
      :min-zoom="0"
      :max-zoom="6"
      :options="{ zoomControl: false, attributionControl: false }"
      @mousemove="onMapMouseMove"
    >
      <l-tile-layer
        :url="tiles"
        no-wrap
        layer-type="base"
        :options="{
          bounds: [
            [0, 0],
            [-256, 256],
          ],
        }"
      ></l-tile-layer>
      <l-marker
        v-if="showFastTravel"
        v-for="i in points.fast_travel"
        :key="`fast-${i[0]}-${i[1]}`"
        :lat-lng="toMapPosition([i[0], i[1]])"
        :options="{ title: $t('map.fastTravel'), alt: $t('map.fastTravel') }"
        @ready="
          (marker) => setMarkerAccessibility(marker, $t('map.fastTravel'))
        "
      >
        <l-icon :icon-url="IconFastTravel" :icon-size="[48, 48]" />
      </l-marker>
      <l-marker
        v-if="showBossTower"
        v-for="i in points.boss_tower"
        :key="`tower-${i[0]}-${i[1]}`"
        :lat-lng="toMapPosition([i[0], i[1]])"
        :options="{ title: $t('map.bossTower'), alt: $t('map.bossTower') }"
        @ready="(marker) => setMarkerAccessibility(marker, $t('map.bossTower'))"
      >
        <l-icon :icon-url="IconBossTower" :icon-size="[48, 48]" />
      </l-marker>
      <l-marker
        v-if="showPlayer"
        v-for="i in visiblePlayerList"
        :key="i.player_uid"
        :lat-lng="toMapPosition([i.location_x, i.location_y])"
        :options="{
          title: `${$t('map.player')}: ${i.nickname}`,
          alt: `${$t('map.player')}: ${i.nickname}`,
        }"
        @ready="
          (marker) =>
            setMarkerAccessibility(marker, `${$t('map.player')}: ${i.nickname}`)
        "
      >
        <l-icon
          :icon-url="IconPlayer"
          :icon-size="[45, 45]"
          :class-name="
            isPlayerOnline(i)
              ? 'player-marker player-marker--online'
              : 'player-marker player-marker--offline'
          "
        />
        <l-tooltip
          :options="{
            direction: 'top',
            permanent: true,
            offset: [0, -15],
            className: isPlayerOnline(i)
              ? 'player-tooltip player-tooltip--online'
              : 'player-tooltip player-tooltip--offline',
          }"
          >{{ i.nickname }}</l-tooltip
        >
        <l-popup :options="{ interactive: true, maxWidth: 400, minWidth: 320 }">
          <article class="map-info-card player-info-card">
            <header class="player-card-header">
              <div
                class="player-avatar"
                :class="{ 'is-offline': !isPlayerOnline(i) }"
                aria-hidden="true"
              >
                {{ playerInitial(i) }}
              </div>
              <div class="player-card-identity">
                <button
                  type="button"
                  class="card-title-link"
                  @click="toPlayer(i.player_uid)"
                >
                  {{ i.nickname || i.player_uid }}
                </button>
                <span
                  class="status-badge"
                  :class="{ 'is-offline': !isPlayerOnline(i) }"
                >
                  <span class="status-dot"></span>
                  {{
                    $t(isPlayerOnline(i) ? "status.online" : "status.offline")
                  }}
                </span>
              </div>
            </header>
            <dl class="player-card-details">
              <div>
                <dt>{{ $t("pal.level") }}</dt>
                <dd>Lv.{{ i.level || 0 }}</dd>
              </div>
              <div>
                <dt>{{ $t("map.guild") }}</dt>
                <dd>
                  <button
                    v-if="playerGuild(i)"
                    type="button"
                    class="inline-link"
                    @click="toGuild(playerGuild(i))"
                  >
                    {{ guildDisplayName(playerGuild(i)) }}
                  </button>
                  <span v-else>{{ $t("map.noGuild") }}</span>
                </dd>
              </div>
            </dl>
            <footer class="card-actions">
              <button
                type="button"
                class="card-action card-action--primary"
                @click="toPlayer(i.player_uid)"
              >
                {{ $t("button.viewPlayer") }}
              </button>
              <button
                v-if="playerGuild(i)"
                type="button"
                class="card-action"
                @click="toGuild(playerGuild(i))"
              >
                {{ $t("button.viewGuild") }}
              </button>
            </footer>
          </article>
        </l-popup>
      </l-marker>
      <template v-if="showBaseCamp">
        <l-marker
          v-for="cluster in clusteredBaseMarkers"
          :key="cluster.key"
          :lat-lng="cluster.position"
          :options="{
            title: clusterLabel(cluster),
            alt: clusterLabel(cluster),
          }"
          @ready="
            (marker) =>
              setMarkerAccessibility(marker, clusterLabel(cluster))
          "
        >
          <l-icon
            :icon-url="IconBase"
            :icon-size="cluster.markers.length > 1 ? [62, 62] : [55, 55]"
          />
          <l-tooltip
            :options="{
              direction: 'top',
              permanent: true,
              offset: [0, -18],
              className: cluster.markers.length > 1 ? 'base-cluster-tooltip' : 'base-name-tooltip',
            }"
          >
            {{ cluster.markers.length > 1 ? cluster.markers.length : cluster.markers[0].displayName }}
          </l-tooltip>
          <l-popup
            :options="{ interactive: true, maxWidth: 460, minWidth: 280 }"
          >
            <section class="map-info-card base-info-card">
              <div v-if="cluster.markers.length > 1" class="cluster-heading">
                {{ $t("map.clusterTitle", { count: cluster.markers.length }) }}
              </div>
              <article
                v-for="marker in cluster.markers"
                :key="marker.key"
                class="base-popup"
              >
                <header class="base-card-header">
                  <div>
                    <span class="card-eyebrow">{{ $t("map.baseCamp") }}</span>
                    <strong class="base-display-name">{{ marker.displayName }}</strong>
                    <button
                      type="button"
                      class="inline-link base-guild-link"
                      @click="toGuild(marker.guild)"
                    >
                      {{ guildDisplayName(marker.guild) }}
                    </button>
                  </div>
                  <span class="base-level">
                    Lv.{{ marker.guild.base_camp_level || 0 }}
                  </span>
                </header>
                <div class="member-section">
                  <div class="section-label">
                    {{
                      $t("guild.memberTitle", {
                        count: marker.guild.players?.length || 0,
                      })
                    }}
                  </div>
                  <div class="member-list">
                    <button
                      v-for="member in marker.guild.players"
                      :key="member.player_uid"
                      type="button"
                      class="member-link"
                      @click="toPlayer(member.player_uid)"
                    >
                      <span class="member-avatar" aria-hidden="true">{{
                        playerInitial(member)
                      }}</span>
                      <span>{{ member.nickname || member.player_uid }}</span>
                    </button>
                  </div>
                </div>
                <div
                  v-if="editingBaseID === marker.camp.id"
                  class="base-alias-editor"
                  @click.stop
                >
                  <n-input
                    v-model:value="aliasName"
                    size="small"
                    maxlength="40"
                    show-count
                    placeholder="例如：北境矿场"
                    @keyup.enter="saveBaseAlias"
                  />
                  <span v-if="aliasError" class="alias-error">{{ aliasError }}</span>
                  <div class="alias-editor-actions">
                    <n-button size="tiny" @click="cancelRenameBase">取消</n-button>
                    <n-button
                      size="tiny"
                      type="primary"
                      :loading="aliasSaving"
                      :disabled="Boolean(aliasError)"
                      @click="saveBaseAlias"
                    >保存</n-button>
                  </div>
                </div>
                <footer class="card-actions">
                  <button
                    type="button"
                    class="card-action card-action--primary"
                    @click="toGuild(marker.guild)"
                  >
                    {{ $t("button.viewGuild") }}
                  </button>
                  <n-button
                    v-if="isAdmin && marker.camp.id && editingBaseID !== marker.camp.id"
                    size="small"
                    secondary
                    @click.stop="beginRenameBase(marker)"
                  >重命名</n-button>
                  <n-button
                    v-if="isAdmin && marker.camp.custom_name && editingBaseID !== marker.camp.id"
                    size="small"
                    text
                    type="error"
                    @click.stop="resetBaseAlias(marker)"
                  >重置</n-button>
                </footer>
              </article>
            </section>
          </l-popup>
        </l-marker>
        <template v-if="zoom >= 5">
          <l-circle
            v-for="marker in rawBaseMarkers"
            :key="`area-${marker.key}`"
            :lat-lng="marker.position"
            :radius="toMapDistance(marker.camp.area)"
          />
        </template>
      </template>
    </l-map>
    <div class="zoom-control">
      <button
        type="button"
        class="zoom-button"
        :aria-label="$t('map.zoomIn')"
        @click="onAddZoom"
      >
        <n-icon size="16"><AddCircle20Filled /></n-icon>
      </button>
      <n-slider
        v-model:value="zoom"
        class="zoom-slider"
        :tooltip="false"
        :theme-overrides="{ railWidthVertical: '3px', handleSize: '14px' }"
        :step="1"
        :min="0"
        :max="6"
        vertical
      />
      <button
        type="button"
        class="zoom-button"
        :aria-label="$t('map.zoomOut')"
        @click="onSubtractZoom"
      >
        <n-icon size="16"><SubtractCircle20Filled /></n-icon>
      </button>
    </div>
    <div class="control" :class="{ 'is-collapsed': controlCollapsed }">
      <button
        type="button"
        class="control-collapse"
        :aria-label="
          $t(controlCollapsed ? 'map.expandPanel' : 'map.collapsePanel')
        "
        :title="$t(controlCollapsed ? 'map.expandPanel' : 'map.collapsePanel')"
        :aria-expanded="!controlCollapsed"
        @click="controlCollapsed = !controlCollapsed"
      >
        <n-icon size="20">
          <ChevronUp v-if="controlCollapsed" />
          <ChevronDown v-else />
        </n-icon>
      </button>
      <div v-if="!controlCollapsed" class="control-content">
        <n-select
          v-model:value="searchTarget"
          :options="searchOptions"
          :placeholder="$t('map.searchTarget')"
          filterable
          clearable
          :aria-label="$t('map.searchTarget')"
          @update:value="focusSearchTarget"
        />
        <div class="visible-summary">
          {{
            $t("map.visibleSummary", {
              players: showPlayer ? visiblePlayerList.length : 0,
              bases: showBaseCamp ? clusteredBaseMarkers.length : 0,
            })
          }}
        </div>
        <div>
          <span>{{ $t("map.showFastTravel") }}</span>
          <n-switch
            v-model:value="showFastTravel"
            :aria-label="$t('map.showFastTravel')"
          />
        </div>
        <div>
          <span>{{ $t("map.showBossTower") }}</span>
          <n-switch
            v-model:value="showBossTower"
            :aria-label="$t('map.showBossTower')"
          />
        </div>
        <div>
          <span>{{ $t("map.showPlayer") }}</span>
          <n-switch
            v-model:value="showPlayer"
            :aria-label="$t('map.showPlayer')"
          />
        </div>
        <div v-if="showPlayer" class="player-filter">
          <span>{{ $t("map.playerVisibility") }}</span>
          <div
            class="visibility-segment"
            role="radiogroup"
            :aria-label="$t('map.playerVisibility')"
          >
            <button
              v-for="option in playerVisibilityOptions"
              :key="option.value"
              type="button"
              role="radio"
              class="visibility-option"
              :class="{ 'is-active': playerVisibility === option.value }"
              :aria-checked="playerVisibility === option.value"
              @click="playerVisibility = option.value"
            >
              {{ option.label }}
            </button>
          </div>
        </div>
        <div>
          <span>{{ $t("map.showBaseCamp") }}</span>
          <n-switch
            v-model:value="showBaseCamp"
            :aria-label="$t('map.showBaseCamp')"
          />
        </div>
        <div class="map-coordinates">
          <span>{{ mousePosition[0] }}, {{ mousePosition[1] }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="less">
.map-view {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 100%;
  overflow: hidden;
  background: #dfe9e4;
}

.leaflet-container {
  background: #102536;
  outline: 0;
}

:deep(.player-marker) {
  transition:
    filter 160ms ease,
    opacity 160ms ease,
    transform 160ms ease;
}

:deep(.player-marker:hover) {
  transform: translateY(-2px);
}

:deep(.player-marker--online) {
  filter: drop-shadow(0 3px 7px rgba(24, 160, 88, 0.5));
}

:deep(.player-marker--offline) {
  filter: grayscale(0.9) saturate(0.35)
    drop-shadow(0 2px 5px rgba(15, 23, 42, 0.35));
  opacity: 0.62;
}

:deep(.player-tooltip) {
  padding: 4px 9px;
  border: 0;
  border-radius: 999px;
  box-shadow: 0 3px 12px rgba(15, 23, 42, 0.2);
  font-size: 12px;
  font-weight: 600;
  line-height: 1.3;
}

:deep(.player-tooltip--online) {
  color: #087443;
  background: rgba(236, 253, 245, 0.96);
}

:deep(.player-tooltip--offline) {
  color: #64748b;
  background: rgba(241, 245, 249, 0.94);
}

:deep(.base-name-tooltip),
:deep(.base-cluster-tooltip) {
  padding: 4px 9px;
  color: #075e4f;
  background: rgba(236, 253, 245, 0.96);
  border: 1px solid rgba(24, 160, 88, 0.18);
  border-radius: 999px;
  box-shadow: 0 3px 12px rgba(15, 23, 42, 0.18);
  font-size: 12px;
  font-weight: 700;
}

:deep(.leaflet-popup-content-wrapper) {
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.22);
  border-radius: 16px;
  box-shadow: 0 18px 44px rgba(15, 23, 42, 0.22);
}

:deep(.leaflet-popup-content) {
  width: auto !important;
  margin: 0;
}

:deep(.leaflet-popup-tip) {
  box-shadow: 3px 3px 8px rgba(15, 23, 42, 0.1);
}

.zoom-control {
  position: absolute;
  bottom: 12px;
  left: 12px;
  z-index: 999;
  display: grid;
  width: 32px;
  grid-template-rows: 22px 78px 22px;
  justify-items: center;
  align-items: center;
  gap: 3px;
  padding: 4px;
  background: rgba(24, 24, 28, 0.68);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 16px;
  box-shadow: 0 10px 28px rgba(2, 8, 23, 0.28);
  backdrop-filter: blur(10px);
}

.zoom-button {
  display: inline-flex;
  width: 22px;
  height: 22px;
  align-items: center;
  justify-content: center;
  padding: 0;
  color: #fff;
  background: transparent;
  border: 0;
  border-radius: 50%;
  cursor: pointer;
}

.zoom-button:hover {
  background: rgba(255, 255, 255, 0.1);
}

.zoom-button:focus-visible {
  outline: 2px solid #63e2a7;
  outline-offset: 1px;
}

.zoom-slider {
  width: 3px !important;
  height: 78px !important;
  margin: 0;
}

.control {
  width: 284px;
  max-width: calc(100% - 40px);
  position: absolute;
  right: 20px;
  bottom: 20px;
  z-index: 999;
  display: flex;
  box-sizing: border-box;
  flex-direction: column;
  padding: 14px;
  color: #fff;
  background: rgba(24, 24, 28, 0.92);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 16px;
  box-shadow: 0 18px 42px rgba(2, 8, 23, 0.34);
  backdrop-filter: blur(14px);
}

.control.is-collapsed {
  width: 52px;
  padding: 6px;
  border-radius: 14px;
}

.control-collapse {
  display: flex;
  width: 100%;
  height: 26px;
  flex: none;
  align-items: center;
  justify-content: center;
  margin: -7px 0 7px;
  padding: 0;
  color: #aeb9c7;
  background: transparent;
  border: 0;
  border-radius: 8px;
  cursor: pointer;
  transition:
    color 150ms ease,
    background 150ms ease;
}

.control-collapse:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.08);
}

.control-collapse:focus-visible {
  outline: 2px solid #63e2a7;
  outline-offset: 1px;
}

.is-collapsed .control-collapse {
  height: 38px;
  margin: 0;
}

.control-content {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.control-content > .n-select {
  width: 100%;
  min-width: 0;
  flex: none;
}

.control-content > div {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 0;
  padding: 7px 0;
}

.visible-summary {
  justify-content: flex-start !important;
  color: #aeb9c7;
  font-size: 12px;
  line-height: 1.45;
}

.player-filter {
  align-items: flex-start !important;
  flex-direction: column;
  gap: 7px !important;
  margin: 2px -4px 4px !important;
  padding: 8px !important;
  background: rgba(255, 255, 255, 0.045);
  border-radius: 11px;
}

.visibility-segment {
  display: grid;
  grid-template-columns: 1fr 1fr;
  width: 100%;
  min-height: 32px;
  padding: 3px;
  background: rgba(0, 0, 0, 0.28);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 9px;
}

.visibility-option {
  min-width: 0;
  padding: 5px 8px;
  color: #aeb9c7;
  background: transparent;
  border: 0;
  border-radius: 6px;
  font: inherit;
  font-size: 12px;
  line-height: 1.3;
  text-align: center;
  cursor: pointer;
  transition:
    color 150ms ease,
    background 150ms ease,
    box-shadow 150ms ease;
}

.visibility-option:hover {
  color: #fff;
}

.visibility-option.is-active {
  color: #fff;
  background: #18a058;
  box-shadow: 0 3px 9px rgba(24, 160, 88, 0.28);
  font-weight: 650;
}

.visibility-option:focus-visible {
  outline: 2px solid #63e2a7;
  outline-offset: 1px;
}

.map-info-card {
  color: #172033;
  background: #fff;
  font-family: inherit;
}

.player-info-card {
  width: min(340px, calc(100vw - 56px));
  padding: 18px;
}

.player-card-header,
.base-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.player-card-header {
  justify-content: flex-start;
}

.player-avatar,
.member-avatar {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  color: #087443;
  background: #dcfce7;
  font-weight: 700;
}

.player-avatar {
  width: 46px;
  height: 46px;
  border: 3px solid #bbf7d0;
  border-radius: 14px;
  font-size: 18px;
}

.player-avatar.is-offline {
  color: #64748b;
  background: #e2e8f0;
  border-color: #cbd5e1;
}

.player-card-identity {
  display: flex;
  min-width: 0;
  flex-direction: column;
  align-items: flex-start;
  gap: 5px;
}

.card-title-link,
.inline-link,
.member-link,
.card-action {
  border: 0;
  font: inherit;
  cursor: pointer;
}

.card-title-link {
  overflow: hidden;
  max-width: 210px;
  padding: 0;
  color: #172033;
  background: transparent;
  font-size: 16px;
  font-weight: 700;
  line-height: 1.3;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-title-link:hover,
.inline-link:hover {
  color: #18a058;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: #087443;
  font-size: 11px;
  font-weight: 600;
}

.status-dot {
  width: 7px;
  height: 7px;
  background: #18a058;
  border-radius: 50%;
  box-shadow: 0 0 0 3px rgba(24, 160, 88, 0.12);
}

.status-badge.is-offline {
  color: #64748b;
}

.status-badge.is-offline .status-dot {
  background: #94a3b8;
  box-shadow: 0 0 0 3px rgba(148, 163, 184, 0.14);
}

.player-card-details {
  display: grid;
  gap: 9px;
  margin: 16px 0;
  padding: 12px 0;
  border-top: 1px solid #eef1f5;
  border-bottom: 1px solid #eef1f5;
}

.player-card-details > div {
  display: grid;
  grid-template-columns: 72px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
}

.player-card-details dt,
.section-label {
  color: #8792a2;
  font-size: 12px;
  font-weight: 500;
}

.player-card-details dd {
  overflow: hidden;
  margin: 0;
  color: #334155;
  font-size: 13px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.inline-link {
  overflow: hidden;
  max-width: 100%;
  padding: 0;
  color: #334155;
  background: transparent;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-actions {
  display: flex;
  gap: 8px;
}

.card-action {
  min-height: 34px;
  flex: 1;
  padding: 7px 12px;
  color: #475569;
  background: #f1f5f9;
  border-radius: 9px;
  font-size: 12px;
  font-weight: 650;
  transition:
    background 150ms ease,
    color 150ms ease;
}

.card-action:hover {
  color: #172033;
  background: #e2e8f0;
}

.card-action--primary {
  color: #fff;
  background: #18a058;
}

.card-action--primary:hover {
  color: #fff;
  background: #12834a;
}

.base-info-card {
  width: min(400px, calc(100vw - 56px));
  max-height: 390px;
  overflow-y: auto;
  padding: 6px 18px 18px;
  scrollbar-color: #cbd5e1 transparent;
  scrollbar-width: thin;
}

.base-info-card::-webkit-scrollbar {
  width: 6px;
}

.base-info-card::-webkit-scrollbar-thumb {
  background: #cbd5e1;
  border-radius: 999px;
}

.cluster-heading {
  position: sticky;
  top: 0;
  z-index: 1;
  margin: 0 -18px 8px;
  padding: 12px 18px 10px;
  color: #64748b;
  background: rgba(255, 255, 255, 0.96);
  border-bottom: 1px solid #eef1f5;
  font-size: 12px;
  font-weight: 650;
}

.base-popup {
  padding-top: 12px;
}

.base-popup + .base-popup {
  margin-top: 14px;
  border-top: 1px solid #e7ebf0;
}

.base-card-header > div {
  min-width: 0;
}

.card-eyebrow {
  display: block;
  margin-bottom: 4px;
  color: #8792a2;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.base-display-name {
  display: block;
  overflow: hidden;
  max-width: 250px;
  color: #172033;
  font-size: 16px;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.base-guild-link {
  display: block;
  margin-top: 4px;
  color: #64748b;
  font-size: 12px;
}

.base-level {
  flex: none;
  padding: 4px 8px;
  color: #087443;
  background: #ecfdf5;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 700;
}

.member-section {
  margin: 14px 0;
}

.section-label {
  margin-bottom: 7px;
}

.member-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.base-alias-editor {
  display: grid;
  gap: 7px;
  margin: 12px 0;
  padding: 10px;
  background: #f6faf8;
  border: 1px solid #dcece5;
  border-radius: 10px;
}

.alias-error {
  color: #d03050;
  font-size: 11px;
}

.alias-editor-actions {
  display: flex;
  justify-content: flex-end;
  gap: 7px;
}

.member-link {
  display: inline-flex;
  max-width: 100%;
  align-items: center;
  gap: 6px;
  padding: 5px 8px 5px 5px;
  color: #334155;
  background: #f4f7f9;
  border-radius: 9px;
  font-size: 12px;
}

.member-link:hover {
  color: #087443;
  background: #ecfdf5;
}

.member-avatar {
  width: 22px;
  height: 22px;
  border-radius: 7px;
  font-size: 10px;
}

.card-title-link:focus-visible,
.inline-link:focus-visible,
.member-link:focus-visible,
.card-action:focus-visible {
  outline: 2px solid #18a058;
  outline-offset: 2px;
}

@media (max-width: 480px) {
  .control {
    right: 16px;
    bottom: 16px;
    width: min(284px, calc(100% - 32px));
    max-width: calc(100% - 32px);
  }

  .player-info-card {
    padding: 15px;
  }
}
</style>
