<script setup>
import { computed } from "vue";

const props = defineProps({
  metadata: { type: Object, default: () => ({}) },
  loading: Boolean,
});
const emit = defineEmits(["refresh"]);

const interval = computed(() => Number(props.metadata?.sync_interval_seconds || 120));
const savedAt = computed(() => props.metadata?.save_file_time || null);
const parsedAt = computed(() => props.metadata?.snapshot_time || null);
const hasSnapshot = computed(() => Boolean(savedAt.value || parsedAt.value));
const dateText = (value) => value
  ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "short", timeStyle: "medium", hour12: false }).format(new Date(value))
  : "等待首次同步";
const nextSyncText = computed(() => {
  if (!parsedAt.value) return "等待首次同步";
  const next = new Date(parsedAt.value).getTime() + interval.value * 1000;
  return next <= Date.now() ? "即将进行" : dateText(next);
});
const statusTitle = computed(() => {
  if (!hasSnapshot.value) return "等待存档同步";
  return props.metadata?.is_stale ? "存档数据可能已过期" : "存档数据已同步";
});
const statusTag = computed(() => {
  if (!hasSnapshot.value) return { type: "default", text: "等待" };
  return props.metadata?.is_stale
    ? { type: "warning", text: "需检查" }
    : { type: "success", text: "正常" };
});
</script>

<template>
  <section class="snapshot-status" aria-label="存档同步状态">
    <div class="status-dot" :class="!hasSnapshot ? 'pending' : metadata.is_stale ? 'stale' : 'fresh'" />
    <div class="status-copy">
      <strong>{{ statusTitle }}</strong>
      <span>游戏存档 {{ dateText(savedAt) }} · PST 解析 {{ dateText(parsedAt) }} · 下次检查 {{ nextSyncText }}（每 {{ interval }} 秒）</span>
    </div>
    <n-tag :type="statusTag.type" size="small" :bordered="false">
      {{ statusTag.text }}
    </n-tag>
    <n-button size="small" quaternary :loading="loading" @click="emit('refresh')">刷新</n-button>
  </section>
</template>

<style scoped>
.snapshot-status {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 10px;
  padding: 11px 14px;
  border: 1px solid var(--ops-line);
  border-radius: 12px;
  background: color-mix(in srgb, var(--ops-panel) 92%, var(--ops-accent) 8%);
}
.status-dot { width: 8px; height: 8px; border-radius: 50%; box-shadow: 0 0 0 4px rgba(47, 125, 104, .11); }
.status-dot.fresh { background: #2f7d68; }
.status-dot.stale { background: #c98932; box-shadow: 0 0 0 4px rgba(201, 137, 50, .12); }
.status-dot.pending { background: #8b9892; box-shadow: 0 0 0 4px rgba(104, 117, 111, .12); }
.status-copy { min-width: 0; }
.status-copy strong, .status-copy span { display: block; }
.status-copy strong { font-size: 13px; font-weight: 600; }
.status-copy span { margin-top: 2px; color: var(--ops-muted); font-size: 12px; font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
@media (max-width: 720px) {
  .snapshot-status { grid-template-columns: auto minmax(0, 1fr) auto; }
  .snapshot-status :deep(.n-button) { grid-column: 2 / 4; justify-self: start; margin-left: -10px; }
  .status-copy span { white-space: normal; line-height: 1.45; }
}
</style>
