export function isPlayerCurrentlyOnline(player, now = Date.now()) {
  if (typeof player?.is_online === "boolean") return player.is_online;
  const lastOnline = new Date(player?.last_online).getTime();
  return Number.isFinite(lastOnline) && now - lastOnline < 80000;
}

export function formatOnlineDuration(seconds) {
  const value = Math.max(0, Math.floor(Number(seconds) || 0));
  const days = Math.floor(value / 86400);
  const hours = Math.floor((value % 86400) / 3600);
  const minutes = Math.floor((value % 3600) / 60);
  const secs = value % 60;
  if (days > 0) return `${days}天 ${hours}小时 ${minutes}分钟`;
  if (hours > 0) return `${hours}小时 ${minutes}分钟`;
  if (minutes > 0) return `${minutes}分钟 ${secs}秒`;
  return `${secs}秒`;
}

export function currentSessionSeconds(player, now = Date.now()) {
  if (!isPlayerCurrentlyOnline(player, now)) return 0;
  const onlineSince = new Date(player?.online_since).getTime();
  if (Number.isFinite(onlineSince) && onlineSince > 0) {
    return Math.max(0, Math.floor((now - onlineSince) / 1000));
  }
  return Math.max(0, Number(player?.current_session_seconds) || 0);
}

export function totalOnlineSeconds(player, now = Date.now(), snapshotAt = now) {
  const elapsed = isPlayerCurrentlyOnline(player, now)
    ? Math.max(0, Math.floor((now - snapshotAt) / 1000))
    : 0;
  return Math.max(0, Number(player?.total_online_seconds) || 0) + elapsed;
}
