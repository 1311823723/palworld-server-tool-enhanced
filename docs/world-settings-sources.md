# Palworld 世界设置字段来源

本分支的 `internal/worldsettings/schema.go` 是随 PST 二进制发布的静态字段表，运行时不会下载或执行第三方配置工具。当前字段表版本为 `palworld-official-1.0.0-2026-07-22`。

## 权威来源

检索日期：2026-07-22。

- Pocketpair 官方 Configuration 文档：<https://docs.palworldgame.com/settings-and-operation/configuration/>
- Pocketpair 官方启动参数文档：<https://docs.palworldgame.com/settings-and-operation/arguments/>
- Pocketpair 官方 PvP 文档：<https://docs.palworldgame.com/settings-and-operation/pvp/>

字段的存在、官方说明、明确列出的枚举值和明确列出的范围以 Pocketpair 文档为准。端口 `1..65535` 属于网络端口的技术约束，并非游戏平衡建议。

## 默认值与冲突处理

官方页面没有为所有字段提供默认值。此类字段的界面默认值参考 MIT 许可的 `pal-conf` 字段表，并在 Schema 的 `source` 中标记为 `pal_conf_inferred`；它们是便于编辑的推定值，不冒充官方保证。官方未给出可靠范围时，后端不擅自添加游戏平衡范围，只校验数据类型和必要技术约束。

当官方文档与 `pal-conf` 冲突时采用以下优先级：

1. 当前 Pocketpair 官方文档；
2. 当前服务器 INI 中已经存在的值；
3. `pal-conf` 推定默认值。

未知字段不进入可编辑 Schema，但普通写回会保留其键、原始值和顺序。备份恢复会先用状态机解析完整内容，再恢复该备份中的全部字段。弃用或保留字段只展示来源信息，后端拒绝通过普通变更请求修改。

## 安全边界

- `AdminPassword`、`ServerPassword` 等密码不回显，不进入差异值、审计或日志。
- 目标文件固定从 PST 的 PalServer 工作目录推导，API 不接受任意文件路径。
- 写入使用同目录临时文件、刷新到磁盘并原子替换；写入前创建权限受限的 `.pst-backups` 备份。
- 应用设置复用 ServerSupervisor 的保存、平滑关服、实际退出确认、启动和防并发状态机。
- 关闭 REST API 会使 PST 的保存、平滑关服、玩家查询和健康检查不可用，界面会给出明确警告。
