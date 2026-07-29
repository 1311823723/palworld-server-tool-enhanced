# PST Production Bridge 使用说明

`PST Production Bridge` 是随 Windows PST Release 提供的服务端 Mod。它在 PalServer 运行时读取据点、工作台、配方和材料，并调用游戏自身的服务器权威生产逻辑创建任务。

它不会修改 `Level.sav`，不会开放额外网络端口，普通玩家也不需要安装客户端 Mod。

## 前置条件

- Windows 本地 PalServer，且 PST 已配置正确的 `PalServer.exe`。
- PalServer 进程由 PST supervisor 管理；外部启动的进程只提供人工安装说明。
- 已由管理员人工安装与当前 Palworld Build 兼容的 UE4SS。
- 使用包含 `extras\PSTProductionBridge` 的正式 Windows Release。

PST 不会下载、安装、升级或覆盖 UE4SS。UE4SS 的来源、版本和升级由服务器管理员自行确认。

## 固定目录

PST 从自身目录读取只读安装源：

```text
<PST 目录>\extras\PSTProductionBridge
```

根据 `PalServer.exe` 自动推导以下目录：

```text
Bridge 目标：<PalServer>\Mods\Workshop\PSTProductionBridge
激活配置：   <PalServer>\Mods\PalModSettings.ini
UE4SS 预期： <PalServer>\Mods\NativeMods\UE4SS
本机 IPC：   <PalServer>\Pal\Saved\PSTProductionBridge
```

浏览器接口不接受下载地址、压缩包、安装源、目标路径或任意命令。

## 一键安装流程

点击“一键安装”并输入确认词 `INSTALL` 后：

1. 调用 Palworld 官方 REST API 保存世界。
2. 创建一份标记为 Bridge 维护来源的世界备份。
3. 调用官方平滑关服接口。
4. 等待 PalServer 进程实际退出。
5. 校验 Release 内 Bridge 清单、文件路径和 SHA-256。
6. 以暂存目录安装 Bridge，并只修改 `PalModSettings.ini` 中必要的激活项。
7. 如果服务器安装前正在运行，按配置等待后重新启动并检查 Bridge 心跳。

如果安装失败，PST 会恢复原 Bridge 目录和原 `PalModSettings.ini`。已停止服务器安装后不会被意外拉起。

“修复”用于恢复与 Release 清单不一致的 Bridge 文件；PST 不会静默覆盖管理员修改过的文件。“安全禁用”只移除激活项，不自动删除 Bridge 文件。

## 生产订单

Bridge 健康后，页面会显示运行时发布的据点、具体工作台、兼容配方和材料：

- `exact`：指定数量。提交时材料不足则拒绝整单。
- `max_available`：提交时按游戏中的实时材料计算最大数量。

同一工作台的订单串行处理。网页预览不是材料预留，Bridge 接受订单时会再次校验据点归属、工作台、配方、解锁状态和材料。

订单状态保存在 `pst.db`。PalServer 或 Bridge 重启后会尝试根据本地映射和工作台队列恢复；无法可靠确认时显示“状态未知”，不会伪造为完成。只有尚未被游戏接受的订单可以取消。

## 状态说明

- `未配置`：尚未配置 Windows PalServer 进程路径。
- `缺少 UE4SS`：需要管理员人工安装兼容依赖。
- `未安装`：依赖正常，可以一键安装 Bridge。
- `文件已修改`：已安装文件与 Release 清单不一致，需要确认后修复。
- `Bridge 离线`：文件已安装但没有收到新心跳。
- `版本不兼容`：协议、Palworld Build 或运行时生产能力未通过校验。
- `运行正常`：目录、哈希、心跳和能力均正常，可以创建订单。

## 安全注意事项

- 所有 Bridge 和生产订单 API 都要求 PST 管理员 JWT。
- Bridge 密钥自动生成并保存在 `pst.db`，不会返回前端。
- IPC 请求只接受固定 JSON 字段，不支持 CMD、PowerShell、Shell、URL 或脚本。
- 安装器拒绝路径穿越、符号链接、清单外文件和哈希异常。
- 不要把 PST 管理接口、Palworld REST API `8212` 或 RCON `25575` 直接暴露公网。

## Palworld 更新后的处理

游戏更新可能改变运行时类、函数或工作台数据结构。Bridge 会在关键能力不可用时关闭订单接收，避免猜测或修改存档。

升级 Palworld 后建议：

1. 先确认 UE4SS 与新 Build 兼容。
2. 在“生产订单”页面重新检测 Bridge。
3. 如果显示“版本不兼容”，不要反复下单或强制修复，等待对应 Bridge 适配版本。
4. 地图、存档备份、进程守护、自动重启、RCON、公会和白名单仍可继续使用。
