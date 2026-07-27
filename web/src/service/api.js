import Service from "./service";

class ApiService extends Service {
  async login(param) {
    let data = param;
    return this.fetch(`/api/login`).post(data).json();
  }

  async getConfigStatus() {
    return this.fetch(`/api/config/status`).get().json();
  }

  async initializeConfig(param) {
    return this.fetch(`/api/config/initialize`).post(param).json();
  }

  async getConfig() {
    return this.fetch(`/api/config`).get().json();
  }

  async updateConfig(param) {
    return this.fetch(`/api/config`).put(param).json();
  }

  async listDirectories(path = "") {
    const query = new URLSearchParams({ path }).toString();
    return this.fetch(`/api/config/directories?${query}`).get().json();
  }

  async testSaveConfig(save) {
    return this.fetch(`/api/config/test/save`).post({ save }).json();
  }

  async testRconConfig(rcon) {
    return this.fetch(`/api/config/test/rcon`).post({ rcon }).json();
  }

  async getServerToolInfo() {
    return this.fetch(`/api/server/tool`).get().json();
  }
  async getServerInfo() {
    return this.fetch(`/api/server`).get().json();
  }
  async getServerMetrics() {
    return this.fetch(`/api/server/metrics`).get().json();
  }
  async sendBroadcast(param) {
    let data = param;
    return this.fetch(`/api/server/broadcast`).post(data).json();
  }
  async shutdownServer(param) {
    let data = param;
    return this.fetch(`/api/server/shutdown`).post(data).json();
  }
  async getServerProcess() {
    return this.fetch(`/api/server/process`).get().json();
  }
  async saveServer() {
    return this.fetch(`/api/server/save`).post().json();
  }
  async startServer() {
    return this.fetch(`/api/server/start`).post().json();
  }
  async restartServer(param) {
    return this.fetch(`/api/server/restart`).post(param).json();
  }
  async stopServer(param) {
    return this.fetch(`/api/server/stop`).post(param).json();
  }
  async setServerWatchdog(param) {
    return this.fetch(`/api/server/watchdog`).post(param).json();
  }
  async getServerUpdate() {
    return this.fetch(`/api/server/update`).get().json();
  }
  async checkServerUpdate() {
    return this.fetch(`/api/server/update/check`).post().json();
  }
  async applyServerUpdate(param) {
    return this.fetch(`/api/server/update/apply`).post(param).json();
  }
  async previewRestartSchedule(param) {
    return this.fetch(`/api/server/restart-schedule/preview`).post(param).json();
  }
  async getRuntimeLogs(param = {}) {
    const query = this.generateQuery(param);
    return this.fetch(`/api/logs?${query}`).get().json();
  }
  async getOperationAudits(param = {}) {
    const query = this.generateQuery(param);
    return this.fetch(`/api/audit?${query}`).get().json();
  }

  async getBaseCamps() {
    return this.fetch(`/api/base-camps`).get().json();
  }
  async getBaseCamp(baseId) {
    return this.fetch(`/api/base-camps/${encodeURIComponent(baseId)}`).get().json();
  }
  async getBaseAliases() {
    return this.fetch(`/api/base-camps/aliases`).get().json();
  }
  async updateBaseAlias(baseId, param) {
    return this.fetch(`/api/base-camps/${encodeURIComponent(baseId)}/alias`).put(param).json();
  }
  async deleteBaseAlias(baseId) {
    return this.fetch(`/api/base-camps/${encodeURIComponent(baseId)}/alias`).delete().json();
  }
  async getBaseWorkPals(baseId, param = {}) {
    const query = this.generateQuery(param);
    return this.fetch(`/api/base-camps/${encodeURIComponent(baseId)}/work-pals?${query}`).get().json();
  }
  async getBaseFeedBoxes(baseId) {
    return this.fetch(`/api/base-camps/${encodeURIComponent(baseId)}/feed-boxes`).get().json();
  }
  async getInventorySummary(param = {}) {
    const query = this.generateQuery(param);
    return this.fetch(`/api/inventory/summary?${query}`).get().json();
  }
  async getInventoryItemLocations(itemId, param = {}) {
    const query = this.generateQuery(param);
    return this.fetch(`/api/inventory/items/${encodeURIComponent(itemId)}/locations?${query}`).get().json();
  }
  async getInventoryContainers(param = {}) {
    const query = this.generateQuery(param);
    return this.fetch(`/api/inventory/containers?${query}`).get().json();
  }
  async getBreedingFarms(param = {}) {
    const query = this.generateQuery(param);
    return this.fetch(`/api/breeding-farms?${query}`).get().json();
  }
  async getBreedingFarm(farmId) {
    return this.fetch(`/api/breeding-farms/${encodeURIComponent(farmId)}`).get().json();
  }
  async getBreedingFarmParents(farmId) {
    return this.fetch(`/api/breeding-farms/${encodeURIComponent(farmId)}/parents`).get().json();
  }
  async getBreedingFarmCakes(farmId) {
    return this.fetch(`/api/breeding-farms/${encodeURIComponent(farmId)}/cakes`).get().json();
  }
  async getBreedingFarmEggs(farmId) {
    return this.fetch(`/api/breeding-farms/${encodeURIComponent(farmId)}/eggs`).get().json();
  }
  async getBreedingCapabilities() {
    return this.fetch(`/api/breeding-farms/capabilities`).get().json();
  }
  async getBreedingNotificationConfig() {
    return this.fetch(`/api/breeding-farms/notification-config`).get().json();
  }
  async updateBreedingNotificationConfig(param) {
    return this.fetch(`/api/breeding-farms/notification-config`).put(param).json();
  }
  async getBreedingEvents(param = {}) {
    const query = this.generateQuery(param);
    return this.fetch(`/api/breeding-farms/events?${query}`).get().json();
  }
  async getUnreadBreedingEvents() {
    return this.fetch(`/api/breeding-farms/events/unread`).get().json();
  }
  async markBreedingEventRead(eventId) {
    return this.fetch(`/api/breeding-farms/events/${encodeURIComponent(eventId)}/read`).post().json();
  }
  async markAllBreedingEventsRead() {
    return this.fetch(`/api/breeding-farms/events/read-all`).post().json();
  }
  async getWorldSettingsSchema() {
    return this.fetch(`/api/world-settings/schema`).get().json();
  }
  async getWorldSettings() {
    return this.fetch(`/api/world-settings`).get().json();
  }
  async validateWorldSettings(param) {
    return this.fetch(`/api/world-settings/validate`).post(param).json();
  }
  async applyWorldSettings(param) {
    return this.fetch(`/api/world-settings/apply`).post(param).json();
  }
  async getWorldSettingsBackups() {
    return this.fetch(`/api/world-settings/backups`).get().json();
  }
  async restoreWorldSettingsBackup(backupId, param) {
    return this.fetch(`/api/world-settings/backups/${encodeURIComponent(backupId)}/restore`).post(param).json();
  }
  async deleteWorldSettingsBackup(backupId) {
    return this.fetch(`/api/world-settings/backups/${encodeURIComponent(backupId)}`).delete().json();
  }

  async getPlayerList(param) {
    const query = this.generateQuery(param);
    return this.fetch(`/api/player?${query}`).get().json();
  }
  async getOnlinePlayerList() {
    return this.fetch(`/api/online_player`).get().json();
  }
  async getPlayer(param) {
    const { playerUid } = param;
    return this.fetch(`/api/player/${playerUid}`).get().json();
  }
  async getPlayerProgress() {
    return this.fetch(`/api/player-progress`).get().json();
  }
  async kickPlayer(param) {
    const { playerUid } = param;
    return this.fetch(`/api/player/${playerUid}/kick`).post().json();
  }
  async banPlayer(param) {
    const { playerUid } = param;
    return this.fetch(`/api/player/${playerUid}/ban`).post().json();
  }
  async unbanPlayer(param) {
    const { playerUid } = param;
    return this.fetch(`/api/player/${playerUid}/unban`).post().json();
  }

  async getGuildList() {
    return this.fetch(`/api/guild`).get().json();
  }
  async getGuild(param) {
    const { adminPlayerUid } = param;
    return this.fetch(`/api/guild/${adminPlayerUid}`).get().json();
  }

  async getWhitelist() {
    return this.fetch(`/api/whitelist`).get().json();
  }

  async addWhitelist(param) {
    let data = param;
    return this.fetch(`/api/whitelist`).post(data).json();
  }

  async removeWhitelist(param) {
    let data = param;
    return this.fetch(`/api/whitelist`).delete(data).json();
  }

  async putWhitelist(param) {
    let data = param;
    return this.fetch(`/api/whitelist`).put(data).json();
  }

  async getRconCommands() {
    return this.fetch(`/api/rcon`).get().json();
  }

  async sendRconCommand(param) {
    let data = param;
    return this.fetch(`/api/rcon/send`).post(data).json();
  }

  async addRconCommand(param) {
    let data = param;
    return this.fetch(`/api/rcon`).post(data).json();
  }

  async putRconCommand(uuid, param) {
    let data = param;
    return this.fetch(`/api/rcon/${uuid}`).put(data).json();
  }

  async removeRconCommand(uuid) {
    return this.fetch(`/api/rcon/${uuid}`).delete().json();
  }

  async getRconTasks() {
    return this.fetch(`/api/rcon/tasks`).get().json();
  }

  async addRconTask(param) {
    return this.fetch(`/api/rcon/tasks`).post(param).json();
  }

  async putRconTask(uuid, param) {
    return this.fetch(`/api/rcon/tasks/${uuid}`).put(param).json();
  }

  async removeRconTask(uuid) {
    return this.fetch(`/api/rcon/tasks/${uuid}`).delete().json();
  }

  async runRconTask(uuid) {
    return this.fetch(`/api/rcon/tasks/${uuid}/run`).post().json();
  }

  async getBackupList(param) {
    const query = this.generateQuery(param);
    return this.fetch(`/api/backup?${query}`).get().json();
  }
  async createBackup() {
    return this.fetch(`/api/backup`).post().json();
  }

  async removeBackup(uuid) {
    return this.fetch(`/api/backup/${uuid}`).delete().json();
  }

  async downloadBackup(uuid) {
    return this.fetch(`/api/backup/${uuid}`).get().blob();
  }
}

export default ApiService;
