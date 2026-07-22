package worldsettings

const SchemaVersion = "palworld-official-1.0.0-2026-07-22"

type Definition struct {
	Key                string   `json:"key"`
	Category           string   `json:"category"`
	Type               string   `json:"type"`
	Default            any      `json:"default"`
	Minimum            *float64 `json:"minimum,omitempty"`
	Maximum            *float64 `json:"maximum,omitempty"`
	Options            []string `json:"options,omitempty"`
	RestartRequired    bool     `json:"restart_required"`
	Secret             bool     `json:"secret"`
	Deprecated         bool     `json:"deprecated"`
	Reserved           bool     `json:"reserved"`
	PerformanceWarning bool     `json:"performance_warning"`
	Dangerous          bool     `json:"dangerous"`
	DescriptionZH      string   `json:"description_zh"`
	DescriptionEN      string   `json:"description_en"`
	Source             string   `json:"source"`
}

func number(value float64) *float64 { return &value }

// Schema is checked into the binary and never fetched at runtime. Defaults
// marked pal_conf_inferred come from the bundled MIT-licensed pal-conf table;
// official_docs entries use only values/ranges explicitly stated by Pocketpair.
var Schema = []Definition{
	{Key: "Difficulty", Category: "basic", Type: "enum", Default: "None", Options: []string{"None"}, RestartRequired: true, DescriptionZH: "难度预设", DescriptionEN: "Difficulty preset", Source: "pal_conf_inferred"},
	{Key: "ServerName", Category: "basic", Type: "string", Default: "Default Palworld Server", RestartRequired: true, DescriptionZH: "服务器名称", DescriptionEN: "Server name", Source: "pal_conf_inferred"},
	{Key: "ServerDescription", Category: "basic", Type: "string", Default: "", RestartRequired: true, DescriptionZH: "服务器说明", DescriptionEN: "Server description", Source: "official_docs"},
	{Key: "DayTimeSpeedRate", Category: "game_balance", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "白天时间流逝倍率", DescriptionEN: "Daytime progression speed", Source: "pal_conf_inferred"},
	{Key: "NightTimeSpeedRate", Category: "game_balance", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "夜晚时间流逝倍率", DescriptionEN: "Nighttime progression speed", Source: "pal_conf_inferred"},
	{Key: "ExpRate", Category: "game_balance", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "经验获取倍率", DescriptionEN: "Experience gain multiplier", Source: "pal_conf_inferred"},
	{Key: "WorkSpeedRate", Category: "game_balance", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "工作速度倍率", DescriptionEN: "Work speed multiplier", Source: "pal_conf_inferred"},

	{Key: "ServerPlayerMaxNum", Category: "server_management", Type: "integer", Default: 32, RestartRequired: true, DescriptionZH: "服务器最大玩家数", DescriptionEN: "Maximum number of players", Source: "official_docs+pal_conf_inferred"},
	{Key: "AdminPassword", Category: "server_management", Type: "password", Default: nil, RestartRequired: true, Secret: true, Dangerous: true, DescriptionZH: "游戏管理员密码", DescriptionEN: "Game administrator password", Source: "official_docs"},
	{Key: "ServerPassword", Category: "server_management", Type: "password", Default: nil, RestartRequired: true, Secret: true, Dangerous: true, DescriptionZH: "加入服务器所需密码", DescriptionEN: "Password required to join", Source: "official_docs"},
	{Key: "PublicIP", Category: "server_management", Type: "string", Default: "", RestartRequired: true, Dangerous: true, DescriptionZH: "社区服务器公开 IP", DescriptionEN: "Community server public IP", Source: "official_docs"},
	{Key: "PublicPort", Category: "server_management", Type: "integer", Default: 8211, Minimum: number(1), Maximum: number(65535), RestartRequired: true, DescriptionZH: "社区服务器公开端口（不改变监听端口）", DescriptionEN: "Community server public port", Source: "official_docs+technical_constraint"},
	{Key: "LogFormatType", Category: "server_management", Type: "enum", Default: "Text", Options: []string{"Text", "Json"}, RestartRequired: true, DescriptionZH: "服务器日志格式", DescriptionEN: "Server log format", Source: "official_docs"},
	{Key: "ChatPostLimitPerMinute", Category: "server_management", Type: "integer", Default: 10, RestartRequired: true, DescriptionZH: "每分钟聊天消息上限", DescriptionEN: "Chat messages allowed per minute", Source: "official_docs+pal_conf_inferred"},
	{Key: "bIsUseBackupSaveData", Category: "server_management", Type: "boolean", Default: true, RestartRequired: true, PerformanceWarning: true, DescriptionZH: "启用游戏自身世界备份", DescriptionEN: "Enable world backups", Source: "official_docs+pal_conf_inferred"},

	{Key: "BaseCampMaxNum", Category: "performance", Type: "integer", Default: 128, RestartRequired: true, PerformanceWarning: true, Dangerous: true, DescriptionZH: "全服据点总数上限", DescriptionEN: "Total bases across the server", Source: "official_docs+pal_conf_inferred"},
	{Key: "BaseCampMaxNumInGuild", Category: "performance", Type: "integer", Default: 4, Maximum: number(10), RestartRequired: true, PerformanceWarning: true, Dangerous: true, DescriptionZH: "每个公会最大据点数", DescriptionEN: "Maximum bases per guild", Source: "official_docs"},
	{Key: "BaseCampWorkerMaxNum", Category: "performance", Type: "integer", Default: 15, Minimum: number(1), Maximum: number(50), RestartRequired: true, PerformanceWarning: true, Dangerous: true, DescriptionZH: "每个据点最大工作帕鲁数量", DescriptionEN: "Maximum Pals per base", Source: "official_docs+pal_conf_inferred"},
	{Key: "ServerReplicatePawnCullDistance", Category: "performance", Type: "float", Default: 15000.0, Minimum: number(5000), Maximum: number(15000), RestartRequired: true, PerformanceWarning: true, Dangerous: true, DescriptionZH: "帕鲁与玩家同步距离（厘米）", DescriptionEN: "Pal replication distance from players", Source: "official_docs"},
	{Key: "ItemContainerForceMarkDirtyInterval", Category: "performance", Type: "float", Default: 1.0, RestartRequired: true, PerformanceWarning: true, DescriptionZH: "容器界面打开时强制重同步间隔", DescriptionEN: "Container forced re-sync interval", Source: "official_docs+pal_conf_inferred"},
	{Key: "MaxBuildingLimitNum", Category: "performance", Type: "integer", Default: 0, Minimum: number(0), RestartRequired: true, PerformanceWarning: true, DescriptionZH: "每位玩家建筑数量上限，0 为不限", DescriptionEN: "Per-player building cap; 0 is unlimited", Source: "official_docs"},
	{Key: "PhysicsActiveDropItemMaxNum", Category: "performance", Type: "integer", Default: nil, Minimum: number(0), RestartRequired: true, PerformanceWarning: true, DescriptionZH: "启用物理行为的掉落物上限", DescriptionEN: "Physics-active dropped item limit", Source: "official_docs"},

	{Key: "PlayerDamageRateAttack", Category: "player", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "玩家造成伤害倍率", DescriptionEN: "Player damage dealt multiplier", Source: "official_docs+pal_conf_inferred"},
	{Key: "PlayerDamageRateDefense", Category: "player", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "玩家受到伤害倍率", DescriptionEN: "Player damage taken multiplier", Source: "official_docs+pal_conf_inferred"},
	{Key: "PlayerStaminaDecreaceRate", Category: "player", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "玩家耐力消耗倍率", DescriptionEN: "Player stamina depletion multiplier", Source: "official_docs+pal_conf_inferred"},
	{Key: "PlayerStomachDecreaceRate", Category: "player", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "玩家饱食度消耗倍率", DescriptionEN: "Player hunger depletion multiplier", Source: "official_docs+pal_conf_inferred"},
	{Key: "bAllowEnhanceStat_Health", Category: "player", Type: "boolean", Default: true, RestartRequired: true, DescriptionZH: "允许分配生命值属性点", DescriptionEN: "Allow allocating points to Health", Source: "official_docs+pal_conf_inferred"},
	{Key: "bAllowEnhanceStat_Attack", Category: "player", Type: "boolean", Default: true, RestartRequired: true, DescriptionZH: "允许分配攻击属性点", DescriptionEN: "Allow allocating points to Attack", Source: "official_docs+pal_conf_inferred"},

	{Key: "PalCaptureRate", Category: "pal", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "帕鲁捕获倍率", DescriptionEN: "Pal capture multiplier", Source: "official_docs+pal_conf_inferred"},
	{Key: "PalSpawnNumRate", Category: "pal", Type: "float", Default: 1.0, RestartRequired: true, PerformanceWarning: true, Dangerous: true, DescriptionZH: "帕鲁生成数量倍率", DescriptionEN: "Pal spawn multiplier", Source: "official_docs+pal_conf_inferred"},
	{Key: "PalDamageRateAttack", Category: "pal", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "帕鲁造成伤害倍率", DescriptionEN: "Pal damage dealt multiplier", Source: "official_docs+pal_conf_inferred"},
	{Key: "PalDamageRateDefense", Category: "pal", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "帕鲁受到伤害倍率", DescriptionEN: "Pal damage taken multiplier", Source: "official_docs+pal_conf_inferred"},
	{Key: "PalEggDefaultHatchingTime", Category: "pal", Type: "float", Default: 72.0, RestartRequired: true, DescriptionZH: "巨大帕鲁蛋孵化时间（小时）", DescriptionEN: "Huge Egg hatching time in hours", Source: "official_docs+pal_conf_inferred"},
	{Key: "PalStomachDecreaceRate", Category: "pal", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "帕鲁饱食度消耗倍率", DescriptionEN: "Pal hunger depletion multiplier", Source: "official_docs+pal_conf_inferred"},

	{Key: "BuildObjectDamageRate", Category: "base_building", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "建筑受到伤害倍率", DescriptionEN: "Damage multiplier to buildings", Source: "official_docs+pal_conf_inferred"},
	{Key: "BuildObjectDeteriorationDamageRate", Category: "base_building", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "建筑劣化速度倍率", DescriptionEN: "Building deterioration multiplier", Source: "official_docs+pal_conf_inferred"},
	{Key: "MonsterFarmActionSpeedRate", Category: "base_building", Type: "float", Default: nil, RestartRequired: true, DescriptionZH: "放牧物品生产速度倍率", DescriptionEN: "Grazing production speed multiplier", Source: "official_docs"},

	{Key: "CollectionDropRate", Category: "drop_collection", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "采集物掉落倍率", DescriptionEN: "Gatherable item drop multiplier", Source: "official_docs+pal_conf_inferred"},
	{Key: "CollectionObjectHpRate", Category: "drop_collection", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "采集物生命值倍率", DescriptionEN: "Gatherable object health multiplier", Source: "official_docs+pal_conf_inferred"},
	{Key: "CollectionObjectRespawnSpeedRate", Category: "drop_collection", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "采集物重生间隔倍率", DescriptionEN: "Gatherable respawn interval multiplier", Source: "official_docs+pal_conf_inferred"},
	{Key: "EnemyDropItemRate", Category: "drop_collection", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "敌人掉落数量倍率", DescriptionEN: "Enemy drop quantity multiplier", Source: "official_docs+pal_conf_inferred"},
	{Key: "ItemWeightRate", Category: "drop_collection", Type: "float", Default: 1.0, RestartRequired: true, DescriptionZH: "物品重量倍率", DescriptionEN: "Item weight multiplier", Source: "official_docs+pal_conf_inferred"},

	{Key: "DeathPenalty", Category: "death_penalty", Type: "enum", Default: "All", Options: []string{"None", "Item", "ItemAndEquipment", "All"}, RestartRequired: true, Dangerous: true, DescriptionZH: "死亡惩罚", DescriptionEN: "Death penalty", Source: "official_docs"},
	{Key: "bPalLost", Category: "death_penalty", Type: "boolean", Default: false, RestartRequired: true, Dangerous: true, DescriptionZH: "死亡后永久失去帕鲁", DescriptionEN: "Permanently lose Pals on death", Source: "official_docs+pal_conf_inferred"},
	{Key: "BlockRespawnTime", Category: "death_penalty", Type: "float", Default: 5.0, Minimum: number(0), RestartRequired: true, DescriptionZH: "死亡后重生冷却（秒）", DescriptionEN: "Respawn cooldown in seconds", Source: "official_docs+pal_conf_inferred"},
	{Key: "RespawnPenaltyDurationThreshold", Category: "death_penalty", Type: "float", Default: nil, Minimum: number(0), RestartRequired: true, DescriptionZH: "重生惩罚持续阈值", DescriptionEN: "Respawn penalty duration threshold", Source: "official_docs"},
	{Key: "RespawnPenaltyTimeScale", Category: "death_penalty", Type: "float", Default: nil, Minimum: number(0), RestartRequired: true, DescriptionZH: "重生冷却倍增倍率", DescriptionEN: "Respawn cooldown multiplier", Source: "official_docs"},

	{Key: "GuildPlayerMaxNum", Category: "guild", Type: "integer", Default: 20, Minimum: number(1), RestartRequired: true, DescriptionZH: "公会最大玩家数", DescriptionEN: "Maximum players per guild", Source: "official_docs+pal_conf_inferred"},
	{Key: "GuildRejoinCooldownMinutes", Category: "guild", Type: "float", Default: 0.0, Minimum: number(0), RestartRequired: true, DescriptionZH: "重新加入公会的冷却分钟数", DescriptionEN: "Guild rejoin cooldown in minutes", Source: "official_docs+pal_conf_inferred"},
	{Key: "bAutoResetGuildNoOnlinePlayers", Category: "guild", Type: "boolean", Default: false, RestartRequired: true, Dangerous: true, DescriptionZH: "长期无人上线时删除公会建筑与据点帕鲁", DescriptionEN: "Reset guild after all members remain offline", Source: "official_docs+pal_conf_inferred"},
	{Key: "AutoResetGuildTimeNoOnlinePlayers", Category: "guild", Type: "float", Default: 72.0, Minimum: number(0), RestartRequired: true, DescriptionZH: "无人上线自动重置等待时间", DescriptionEN: "Offline duration before guild reset", Source: "official_docs+pal_conf_inferred"},

	{Key: "bEnableFastTravel", Category: "features", Type: "boolean", Default: true, RestartRequired: true, DescriptionZH: "启用快速旅行", DescriptionEN: "Enable fast travel", Source: "official_docs+pal_conf_inferred"},
	{Key: "bEnableFastTravelOnlyBaseCamp", Category: "features", Type: "boolean", Default: false, RestartRequired: true, DescriptionZH: "仅允许据点间快速旅行", DescriptionEN: "Restrict fast travel to bases", Source: "official_docs+pal_conf_inferred"},
	{Key: "bEnableInvaderEnemy", Category: "features", Type: "boolean", Default: true, RestartRequired: true, DescriptionZH: "启用入侵敌人", DescriptionEN: "Enable invaders", Source: "official_docs+pal_conf_inferred"},
	{Key: "bAllowGlobalPalboxExport", Category: "features", Type: "boolean", Default: true, RestartRequired: true, DescriptionZH: "允许导出到全局帕鲁终端", DescriptionEN: "Allow Global Palbox export", Source: "official_docs+pal_conf_inferred"},
	{Key: "bAllowGlobalPalboxImport", Category: "features", Type: "boolean", Default: false, RestartRequired: true, DescriptionZH: "允许从全局帕鲁终端导入", DescriptionEN: "Allow Global Palbox import", Source: "official_docs+pal_conf_inferred"},
	{Key: "DenyTechnologyList", Category: "features", Type: "technology_list", Default: []string{}, RestartRequired: true, DescriptionZH: "禁用的科技 ID 列表", DescriptionEN: "Disabled technology IDs", Source: "official_docs"},

	{Key: "bIsPvP", Category: "pvp", Type: "boolean", Default: false, RestartRequired: true, Dangerous: true, DescriptionZH: "启用 PvP 试验功能", DescriptionEN: "Enable experimental PvP", Source: "official_docs"},
	{Key: "bEnablePlayerToPlayerDamage", Category: "pvp", Type: "boolean", Default: false, RestartRequired: true, Dangerous: true, DescriptionZH: "允许玩家互相伤害", DescriptionEN: "Enable player-to-player damage", Source: "official_docs"},
	{Key: "bEnableDefenseOtherGuildPlayer", Category: "pvp", Type: "boolean", Default: false, RestartRequired: true, Dangerous: true, DescriptionZH: "据点帕鲁攻击敌对公会玩家", DescriptionEN: "Allow base Pals to defend against other guilds", Source: "official_docs"},
	{Key: "bCanPickupOtherGuildDeathPenaltyDrop", Category: "pvp", Type: "boolean", Default: false, RestartRequired: true, Dangerous: true, DescriptionZH: "允许拾取其他公会死亡掉落", DescriptionEN: "Allow picking up other guild death drops", Source: "official_docs"},

	{Key: "RandomizerType", Category: "randomization", Type: "enum", Default: "None", Options: []string{"None", "Region", "All"}, RestartRequired: true, DescriptionZH: "帕鲁生成随机化模式", DescriptionEN: "Pal spawn randomization mode", Source: "official_docs"},
	{Key: "RandomizerSeed", Category: "randomization", Type: "string", Default: "", RestartRequired: true, DescriptionZH: "帕鲁生成随机种子", DescriptionEN: "Randomizer seed", Source: "official_docs"},
	{Key: "bIsRandomizerPalLevelRandom", Category: "randomization", Type: "boolean", Default: false, RestartRequired: true, DescriptionZH: "完全随机化野生帕鲁等级", DescriptionEN: "Fully randomize wild Pal levels", Source: "official_docs"},

	{Key: "RESTAPIEnabled", Category: "rest_rcon", Type: "boolean", Default: false, RestartRequired: true, Dangerous: true, DescriptionZH: "启用官方 REST API；关闭后 PST 多项功能不可用", DescriptionEN: "Enable the official REST API", Source: "official_docs+pal_conf_inferred"},
	{Key: "RESTAPIPort", Category: "rest_rcon", Type: "integer", Default: 8212, Minimum: number(1), Maximum: number(65535), RestartRequired: true, Dangerous: true, DescriptionZH: "官方 REST API 监听端口", DescriptionEN: "Official REST API port", Source: "official_docs+technical_constraint"},
	{Key: "RCONEnabled", Category: "rest_rcon", Type: "boolean", Default: false, RestartRequired: true, Dangerous: true, DescriptionZH: "启用 RCON（存在安全风险）", DescriptionEN: "Enable RCON", Source: "official_docs+pal_conf_inferred"},
	{Key: "RCONPort", Category: "rest_rcon", Type: "integer", Default: 25575, Minimum: number(1), Maximum: number(65535), RestartRequired: true, Dangerous: true, DescriptionZH: "RCON 端口", DescriptionEN: "RCON port", Source: "official_docs+technical_constraint"},
	{Key: "CrossplayPlatforms", Category: "rest_rcon", Type: "platform_list", Default: []string{"Steam", "Xbox", "PS5", "Mac"}, Options: []string{"Steam", "Xbox", "PS5", "Mac"}, RestartRequired: true, DescriptionZH: "允许连接的平台", DescriptionEN: "Platforms allowed to connect", Source: "official_docs"},

	{Key: "BanListURL", Category: "advanced", Type: "string", Default: "https://b.palworldgame.com/api/banlist.txt", RestartRequired: true, Dangerous: true, DescriptionZH: "封禁列表 URL", DescriptionEN: "Ban list URL", Source: "pal_conf_inferred"},
	{Key: "bAllowClientMod", Category: "advanced", Type: "boolean", Default: true, RestartRequired: true, Dangerous: true, DescriptionZH: "允许启用 Mod 的客户端加入", DescriptionEN: "Allow modded clients", Source: "official_docs+pal_conf_inferred"},
	{Key: "AllowConnectPlatform", Category: "deprecated_reserved", Type: "string", Default: nil, RestartRequired: true, Deprecated: true, Reserved: true, DescriptionZH: "当前版本不可用，请使用 CrossplayPlatforms", DescriptionEN: "Unavailable; use CrossplayPlatforms", Source: "official_docs"},
}

func SchemaByKey() map[string]Definition {
	result := make(map[string]Definition, len(Schema))
	for _, definition := range Schema {
		result[definition.Key] = definition
	}
	return result
}

func PublicSchema() []Definition {
	result := make([]Definition, len(Schema))
	copy(result, Schema)
	for i := range result {
		if result[i].Secret {
			result[i].Default = nil
		}
	}
	return result
}
