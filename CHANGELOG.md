# Changelog

本文件记录 `feature/windows-server-supervisor` 分支相对上游 PST 的增强内容。项目继续按照 Apache License 2.0 发布，并保留原作者归属。

## [Unreleased]

### Added

- Windows 本地 `PalServer.exe` 并发安全进程管理器与明确状态机。
- 保存世界、启动、平滑停服、平滑重启和崩溃自动重启 API。
- 五分钟窗口内连续失败五次的默认崩溃循环保护，可由管理员重新启用。
- PST 启动时检测外部 `PalServer.exe` / `PalServer-Win64-Shipping-Cmd.exe` 进程。
- `config.db` 中的 `server_process` 配置组和严格可执行文件/参数校验。
- 桌面端与移动端共用的服务器进程状态卡、操作按钮和确认弹窗。
- fake launcher 驱动的 supervisor 状态机、并发、鉴权、配置与敏感字段测试。

### Changed

- 官方 REST API 客户端增加 `/v1/api/save` 封装，平滑流程复用现有认证和 HTTP 客户端。
- 配置查询不再向前端返回 RCON 和 REST API 密码；前端提交空密码时保留原值。
- 原生 Release 工作流在打包前运行完整 Go 测试。

### Security

- 所有新增进程控制路由均位于现有 JWT 管理员路由组。
- 不提供任意命令执行能力，只允许白名单 PalServer 文件名和安全参数数组。
