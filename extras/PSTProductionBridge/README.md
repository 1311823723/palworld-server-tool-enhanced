PST Production Bridge 0.1.1

这是 PST 专服增强版随包提供的服务端 Bridge。

安全边界：
1. 仅从 Pal\Saved\PSTProductionBridge 读取固定结构 JSON 请求。
2. 不监听网络端口，不执行 CMD、PowerShell 或任意 Shell 命令。
3. 不读取或修改 Palworld 存档。
4. 下单通过 Palworld 自带的 UPalUIConvertItemModel 服务器生产逻辑完成。
5. 当前游戏 Build 无法验证运行时模型时，Bridge 会关闭订单能力。

PST 不携带、不安装、不升级 UE4SS。请自行确认 UE4SS 与当前 Palworld
Dedicated Server 版本兼容。

启动成功后，Palworld 官方 Mod Loader 应创建：

`Mods\ManagedMods\PSTProductionBridge\InstallManifest.json`

并把 Lua 部署到：

`Mods\NativeMods\UE4SS\Mods\PSTProductionBridge`
