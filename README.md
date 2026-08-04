<h1 align='center'>幻兽帕鲁服务器管理工具 Enhanced</h1>

<p align="center">
   <strong>简体中文</strong> | <a href="/docs/README.en.md">English</a> | <a href="/docs/README.ja.md">日本語</a>
</p>

<p align='center'>
  通过可视化界面及 REST 接口管理幻兽帕鲁专用服务器，基于 SAV 存档解析、官方 REST API 与 RCON 实现。
</p>

<p align='center'>
<img alt="GitHub Repo stars" src="https://img.shields.io/github/stars/1311823723/palworld-server-tool-enhanced?style=for-the-badge">&nbsp;&nbsp;
<img alt="Go" src="https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white">&nbsp;&nbsp;
<img alt="Python" src="https://img.shields.io/badge/Python-FFD43B?style=for-the-badge&logo=python&logoColor=blue">&nbsp;&nbsp;
<img alt="Vue" src="https://img.shields.io/badge/Vue%20js-35495E?style=for-the-badge&logo=vue.js&logoColor=4FC08D">
</p>

![PC](./docs/img/pst-zh-1.png)

## 功能

- 玩家、公会、帕鲁和背包数据查看
- 服务器信息、指标和在线玩家列表
- 踢出、封禁、广播和平滑关闭服务器
- 可视化地图与白名单管理
- 自定义及定时 RCON 命令
- 存档定时同步、自动备份与备份管理
- 桌面端和移动端自适应界面
- 统一中文运营台：总览、玩家、地图、帕鲁管理、据点、库存、配种农场、服务器运维和世界设置
- 移动端底部导航与完整页面滚动
- 管理模式中的可视化 PST 配置
- 按据点查看工作帕鲁、健康状态、当前工作和饲料箱快照
- 为每个据点设置持久化中文名称，并统一用于库存、配种和游戏内提醒
- 管理员只读全服库存聚合、物品来源和容器位置查询
- 具有备份、校验、平滑重启、健康检查和失败回滚的 `PalWorldSettings.ini` 编辑器
- 管理员配种农场页面、父母帕鲁与专属蛋糕查看、游戏内广播、浏览器提醒和持久化通知中心
- 标准五段 Cron 自动重启、SteamCMD 安全更新、PST 日志和操作审计
- 玩家等级、经验、图鉴、探索、首领与科技进度的只读存档解析
- NapCatQQ + OneBot 11 群机器人：查询状态、玩家、库存、据点、配种和备份，管理员可二次确认后改名或控制 PalServer
- 可选 DeepSeek 自然语言增强；未配置或调用失败时全部基础 QQ 命令仍可使用

业务数据保存在 `pst.db`，PST 配置和管理员凭据单独保存在 `config.db`。清理或重置配置不会影响玩家、公会、RCON 和备份记录。

## 本分支新增：Windows 本地进程管理

本分支在保留原项目玩家、公会、RCON、备份、定时任务和配置中心能力的基础上，增加 Windows 本地 Palworld Dedicated Server 进程管理：

- 从 PST 管理界面启动固定路径的 `PalServer.exe`
- 通过官方 REST API 保存世界、平滑停服和平滑重启
- 服务器意外退出后延迟自动重启，并在短时间连续失败时触发崩溃循环保护
- 管理员手动停服后保持关闭，不会被守护程序再次拉起
- 按 PST 主机本地时间执行每天、每隔数天、每周或每月自动平滑重启，并显示下次与最近执行状态
- 在桌面端和移动端查看 PID、运行时间、守护状态、最近退出和错误
- PST 重启时识别已经运行的 `PalServer.exe` 或 `PalServer-Win64-Shipping-Cmd.exe`，避免重复启动

### 配置 Windows PalServer

进入管理模式，打开“PST 配置”中的“Windows PalServer 进程”，填写：

1. 开启“本机进程管理”。
2. 将“PalServer.exe 路径”设为服务器实际路径，例如 `D:\Program Files\Steam\steamapps\common\PalServer\PalServer.exe`。
3. 工作目录可以留空；留空时使用可执行文件所在目录。
4. 每条启动参数单独添加，例如 `-port=8211`、`-players=8`、`-logformat=text`。不要输入一整条 CMD 命令。
5. 按需开启崩溃守护，并配置重启等待时间、连续失败上限和统计窗口。
6. 如需定时重启，开启“自动重启计划”，选择“每天”“每隔几天”“每周一次”或“每月一次”，再设置日期和时间。例如选择“每天”和 `04:00`，PST 会在每天凌晨 4 点开始平滑重启。普通用户无需编写 Cron 表达式。

保存后 supervisor 会立即加载新配置，但不会擅自重启当前正在运行的游戏服务器。回到管理员概览的“服务器进程”卡片即可保存世界、启动、平滑重启、平滑停服或切换守护状态。重启会等待旧进程实际退出，再从退出时刻开始计算重启延迟，不会同时启动两个实例。

自动重启使用运行 PST 的 Windows 主机本地时区。“每隔几天”从配置的首次执行日期开始计算；“每月”选择 29 至 31 日而当月没有该日期时，会在当月最后一天执行。若到点时服务器已被管理员保持停服，或正在执行其他启动、停服或重启操作，本次计划会安全跳过，不会强制拉起服务器；跳过原因会显示在进程卡片中。

Windows 是本功能的首要支持平台。Linux 和 macOS 仍可使用 PST 原有管理功能，但尝试启动本地 PalServer 进程时会收到明确的 `unsupported platform` 错误。Docker 部署也不应使用宿主机 Windows 进程管理，应继续通过 REST API、RCON 或 `pst-agent` 管理独立游戏服务器。

### QQ 机器人

管理员登录后可打开“QQ 机器人”，按页面步骤连接同一台 Windows 主机上的 NapCatQQ 正向 WebSocket。NapCat 监听地址必须是 `127.0.0.1` 或 `::1`，PST 主动连接，不新增公网端口。OneBot Token 只保存在本机 `config.db`，并只作为本机 WebSocket 鉴权头发送给 NapCat；配置接口不会回显 Token。

不配置 DeepSeek 时，机器人仍可处理服务器状态、在线玩家及在线时间、库存、据点、异常工作帕鲁、配种提醒、备份和自动重启计划等基础命令。明确配置的管理员 QQ 还可发起据点改名、PalServer 启动、平滑重启和平滑停服；写操作必须由同一 QQ 在同一会话内使用六位验证码二次确认。这里的“关机”只表示停止 PalServer 并保持关闭，机器人不能关闭 Windows 或 PST，也不能执行 RCON、CMD、PowerShell、SteamCMD、世界设置和任意文件操作。

DeepSeek 仅用于本地规则无法确定意图时的可选增强。PST 只向 DeepSeek 官方 HTTPS API 发送经过脱敏的必要文本和查询结果，不发送 OneBot Token、JWT、密码、IP、本机路径或玩家技术 ID。API Key 清除或额度不足不会影响基础命令。完整安装步骤、命令和权限说明见 [QQ 机器人配置说明](./docs/qq-bot.md)。

### 服务器运维中心

管理员登录后可从“服务器运维”统一管理以下能力：

- **进程控制**：保存世界、启动、平滑重启、保持停服和崩溃守护。
- **自动重启**：普通用户可选择每天、每隔数日、每周或每月；高级用户可填写标准五段 Cron，例如 `0 4 * * *` 表示按 PST 主机时区每天 04:00 执行。页面会校验表达式并预览未来三次执行时间。
- **服务器更新**：配置严格的 `steamcmd.exe` 路径后检查或应用 Palworld Dedicated Server 更新。App ID 固定为 `2394010`，PST 不接受自定义 SteamCMD 命令。应用更新必须输入 `UPDATE`，并依次执行保存、备份、平滑关服、更新和重新启动。
- **存档备份**：查看、下载和删除自动备份，也可手动创建备份；运行中的服务器会先调用官方保存接口。
- **RCON、日志与审计**：保留 RCON 命令和定时任务，并按需查看脱敏后的 PST 日志与管理员操作记录。

SteamCMD 更新失败时，PST 会暂停守护并保持明确的“更新失败”状态，避免对可能不完整的服务器文件进行无限启动。管理员应先检查日志、修复 SteamCMD 或网络问题，再重试更新或手动启动。

### 从包含 Production Bridge 的测试版恢复

生产订单功能已暂时下线，新版本不会再安装或加载 Production Bridge。如果服务器曾安装过测试版 Bridge，请先停止 PalServer，再用文本编辑器完成以下清理：

1. 打开 `PalServer\Mods\PalModSettings.ini`，只删除 `ActiveModList=PSTProductionBridge` 这一行，保留其他 Mod 配置。
2. 删除 `PalServer\Mods\NativeMods\UE4SS\Mods\PSTProductionBridge` 文件夹。
3. 如果 `PalModSettings.ini` 配置了 `WorkshopRootDir`，进入该目录并删除其中的 `PSTProductionBridge` 文件夹。
4. 重新启动 PalServer，确认服务器稳定后再启动 PST 的自动同步。

不需要删除 UE4SS，也不要删除其他 Mod、`config.db`、`pst.db` 或游戏存档。

### 玩家成长进度

玩家页面中的“成长进度”标签从玩家存档只读解析等级、经验、帕鲁数量、图鉴发现与捕获、快速传送点、探索区域、首领进度、科技点和已解锁配方。页面同时显示本次持续在线时长和累计在线时长。

不同 Palworld 版本的存档字段可能变化。接口会随响应返回 `capabilities` 和解析时间；未经当前真实存档验证或无法解析的字段显示“当前存档无法解析”，不会伪造为 `0`。本功能不会修改玩家存档。

### 进程控制安全注意事项

- 进程控制接口全部要求 PST 管理员 JWT，不提供匿名访问。
- PST 只允许启动配置中的 `PalServer.exe` 或 `PalServer-Win64-Shipping-Cmd.exe`，不提供任意 CMD、PowerShell 或 Shell 执行接口。
- 配置中的启动参数是字符串数组；`&`、`|`、`>`、`<`、`cmd.exe` 和 `powershell.exe` 会被后端拒绝。
- 配置查询不会返回 RCON 密码、REST API 密码、JWT 或其他敏感凭据。示例密码统一使用 `YOUR_ADMIN_PASSWORD`。
- 不要把游戏 REST API 端口 `8212`、RCON 端口 `25575` 或 PST 进程控制 API 直接暴露到公网。
- 通过 Cloudflare Tunnel 发布 PST 时，除 PST 登录外必须再启用 Cloudflare Access 等额外身份验证，并限制可访问用户。
- 手机远程访问推荐使用 [Cloudflare Tunnel 远程部署说明](./docs/remote-access-cloudflare.md)，Tunnel 只连接 PST 的 `127.0.0.1:8080`，不需要开放路由器端口。

## 本分支新增：据点、库存和世界设置

存档同步现在会额外生成一个规范化快照，原子写入现有 `pst.db`。桌面端和移动端均可打开“据点工作帕鲁”，按据点查看工作帕鲁数量、生命、饱食度、SAN、异常状态、当前工作和饲料箱；管理员还可打开“全服库存”，查看玩家与据点容器中各物品的聚合数量，并按需加载具体位置。

“据点管理”允许管理员为每个据点设置 1 至 40 个字符的自定义名称。名称以稳定的据点 ID 保存在 `pst.db`，不会写回或修改 Palworld 存档，重新解析和重启 PST 后仍然有效。工作帕鲁、库存位置、配种农场、历史事件和游戏内产蛋提醒都会使用该名称；存档原名仍保留在接口中，便于诊断。世界换档后失效的名称不会被自动删除，可在据点管理页面手动清理。

这些页面显示的是最近一次 `Level.sav` 解析结果，并非游戏内存实时数据。默认每 120 秒检查一次存档，也可以在 PST 配置中心调整同步间隔；实际延迟还取决于 Palworld 何时写入存档。页面会显示存档时间、解析时间、过期状态、解析警告和当前解析能力；解析器无法可靠取得的字段返回 `null` 或能力说明，不会伪造为零。升级后请确认 `sav_cli` 同步成功，旧的玩家、公会接口行为保持不变。

管理员可以从“世界设置”页面编辑固定路径：

```text
<PalServer 工作目录>\Pal\Saved\Config\WindowsServer\PalWorldSettings.ini
```

字段表随 PST 发布，不在运行时从网络下载。应用流程为：校验差异、保存世界、平滑关服、等待 PalServer 实际退出、创建 `.pst-backups` 备份、同目录原子替换 INI、启动服务器并检查官方 REST API。新配置启动或健康检查失败时，PST 会恢复旧 INI 和旧的 REST/RCON 连接配置，并只尝试一次旧配置启动。未知 INI 字段会在普通编辑和写回时原样保留；恢复备份则恢复该备份的完整内容。

密码字段只显示“已设置/未设置”，不会返回原值。输入为空表示保持不变，必须显式勾选才能清空。设置接口、库存接口和备份操作均要求管理员 JWT；库存没有修改、转移或删除接口。字段来源和冲突处理见 [世界设置字段来源说明](./docs/world-settings-sources.md)。

可使用不输出玩家、公会、物品或密码内容的诊断脚本检查存档结构：

```bash
python3 script/diagnose_save.py /path/to/Level.sav
```

## 本分支新增：配种农场监控

管理员可从桌面端或移动端打开“配种农场”，按据点查看严格关联的配种农场、两个真实分配槽中的父母帕鲁、农场专属蛋糕容器和已经产出的待拾取蛋。可以按据点、具体农场或二次确认后的全部农场开启提醒，并在站内通知中心查看历史、标记已读。游戏内提醒通过已配置的 Palworld 官方 REST API 广播到玩家界面，不要求 PST 页面保持打开；浏览器通知仍需要用户主动授权。

提醒比较连续两次成功存档快照中的稳定蛋实例集合，事件基线、去重键和已读状态持久化在 `pst.db`，页面刷新或 PST 重启不会把同一枚蛋重复记为新事件。首次开启默认只建立基线，不提醒已经存在的蛋；可由管理员显式开启一次性现有蛋提醒。

这些数据不是实时遥测。提醒会等到游戏完成下一次正常保存并由 PST 解析后才出现，因此可能晚于游戏实际产蛋时间；本功能不会为了追求实时性高频调用官方保存接口。当前验证样本无法可靠取得蛋种、配种进度、暂停原因或预计完成时间，这些字段保持未知，不根据父母组合或附近物体猜测。

详细配置和限制见 [配种农场监控说明](./docs/breeding-farm-monitor.md)，真实字段与能力矩阵见 [配种农场存档字段](./docs/breeding-farm-save-fields.md)。

> [!NOTE]
> 如果您需要幻兽帕鲁服务器或工具搭建交流，或者需要闭源付费定制功能开发，请加入幻兽帕鲁服务器管理交流群。

![加QQ群](./docs/img/add_group.jpg)

## 功能截图

### 桌面端

|                              |                              |
| :--------------------------: | :--------------------------: |
| ![](./docs/img/pst-zh-2.png) | ![](./docs/img/pst-zh-3.png) |
| ![](./docs/img/pst-zh-4.png) | ![](./docs/img/pst-zh-5.png) |

### 移动端

<p align="center">
<img src="./docs/img/pst-zh-m-1.png" width="24%" /><img src="./docs/img/pst-zh-m-2.png" width="24%" /><img src="./docs/img/pst-zh-m-3.png" width="24%" /><img src="./docs/img/pst-zh-m-4.png" width="24%" />
</p>

## 开启 REST API 与 RCON

PST 需要游戏服务器开启官方 REST API；自定义 RCON 功能还需要开启 RCON。[RCON 命令参考](./docs/rconCommand_zh.txt)

关闭游戏服务器后，可通过 [Pal-Conf](https://pal-conf.bluefissure.com/) 修改 `PalWorldSettings.ini` 或 `WorldOption.sav`，先设置游戏服务器的管理员密码，再启用 RCON 和 REST API。

![ADMIN](./docs/img/admin-zh.png)

![RCON_REST](./docs/img/rest-rcon-zh.png)

## 安装部署

解析 `Level.sav` 会在短时间内使用约 1GB～3GB 内存，请确保运行环境有足够资源。

### 文件部署

1. 从本增强版的 [GitHub Releases](https://github.com/1311823723/palworld-server-tool-enhanced/releases) 下载对应系统和架构的压缩包并解压。
2. Linux/macOS 给 `pst` 和 `sav_cli` 增加执行权限并运行 `./pst`；Windows 双击 `start.bat`，或在 PowerShell 中运行 `.\pst.exe`。
3. 浏览器访问 `http://127.0.0.1:8080` 或 `http://{服务器 IP}:8080`，创建管理员并在 Web 弹窗中完成配置。

首次启动使用端口 `8080`。如果该端口已被占用，可以通过命令行参数或环境变量覆盖监听端口：

```bash
# Linux/macOS：命令行参数
./pst --port 18080

# Linux/macOS：环境变量
PST_PORT=18080 ./pst
```

Windows 可以在 PowerShell 中临时指定端口：

```powershell
.\pst.exe --port 18080
```

需要固定双击启动时的默认端口，可以用文本编辑器打开解压目录中的 `start.bat`，将启动命令修改为：

```bat
start cmd /k .\pst.exe --port 18080
```

端口优先级为 `--port` > `PST_PORT` > `config.db` > 默认值 `8080`。使用命令行参数或环境变量启动时，最终端口和 `port_source` 覆盖来源会同步写入 `config.db`，确保 `sav_cli` 等内部服务读取到的端口与实际监听端口一致。覆盖存在期间，配置中心会显示实际端口和覆盖来源，并禁用端口输入框；配置接口也会拒绝修改端口。每次启动都会重新计算来源；移除覆盖后，`config.db` 会保留最后一次覆盖的端口并将 `port_source` 置空，此时可以再从 Web 修改。

如在 Web 中修改端口、TLS 或其他启动设置，保存后重启 PST。

> [!IMPORTANT]
> 除专用于启动端口覆盖的 `--port` 和 `PST_PORT` 外，PST 不再读取 `config.yaml`、`-config` 参数或其他 PST 配置环境变量。升级用户请在 Web 配置弹窗中手动复制旧值，确认后删除旧文件和变量。

### Docker 单体部署

先创建需要持久化的数据库文件：

```bash
touch pst.db config.db
```

运行容器，并把游戏存档目录映射到容器内：

```bash
docker run -d --name pst \
  -p 8080:8080 \
  -v /path/to/your/Pal/Saved:/game \
  -v ./backups:/app/backups \
  -v ./pst.db:/app/pst.db \
  -v ./config.db:/app/config.db \
  jokerwho/palworld-server-tool:latest
```

需要覆盖容器内的 PST 监听端口时，同时调整端口映射并传入 `PST_PORT`，例如：

```bash
docker run -d --name pst \
  -p 18080:18080 \
  -e PST_PORT=18080 \
  -v /path/to/your/Pal/Saved:/game \
  -v ./backups:/app/backups \
  -v ./pst.db:/app/pst.db \
  -v ./config.db:/app/config.db \
  jokerwho/palworld-server-tool:latest
```

进入 Web 配置后选择“本机目录”，填写或浏览选择容器内的 `/game`。RCON 和 REST API 地址必须是容器能够访问的游戏服务器地址。

`pst.db` 保存业务数据，`config.db` 只保存配置和管理员凭据，两者应分别持久化。需要重置管理员和全部配置时，停止 PST、删除 `config.db` 后重新启动即可。

### Agent 部署

游戏服务器与 PST 不在同一主机时，先在游戏服务器侧启动 `pst-agent`：

```bash
docker run -d --name pst-agent \
  -p 8081:8081 \
  -v /path/to/your/Pal/Saved:/game \
  -e SAVED_DIR="/game" \
  jokerwho/palworld-server-tool-agent:latest
```

再按上面的方式启动 PST，不需要为 PST 容器传入配置环境变量。进入 Web 配置，选择“pst-agent”，填写 `http://游戏服务器IP:8081/sync`，并配置 RCON 与 REST API 地址。

`pst-agent` 自身仍使用命令行参数或 `SAVED_DIR` 指定存档目录，详细操作见 [pst-agent 部署教程](./docs/README.agent.md)。

## 首次进入与配置

1. 访问 PST Web 页面。首次访问必须创建管理模式密码，此操作只允许成功一次。该密码只保护 PST Web 面板，不是游戏服务器的 `AdminPassword`。
2. 首位访问者会成为管理员。如果被他人抢先设置，停止 PST，删除 `config.db` 后重新启动；`pst.db` 不受影响。
3. 创建管理员后会自动打开配置弹窗。选择“本机目录”时，可以直接浏览 PST 所在主机的文件系统；跨主机请选择“pst-agent”并填写同步 URL。
4. 存档和 RCON 配置组会显示“未配置 / 报错 / 正常”状态；RCON 可通过官方只读 `Info` 命令测试连接，不会修改游戏服务器状态。
5. 填写 RCON、REST API、同步、备份和自动化选项并保存。存档来源、RCON、REST、消息、管理选项和管理员密码等立即生效；只有 Web 监听/TLS 与定时任务周期变化需要重启，页面会列出具体项目。
6. 后续从管理模式的“PST 配置”入口修改。管理员密码更换后，旧登录令牌立即失效。

所有持久化配置均写入当前工作目录的 `config.db`。启动时通过 `--port` 或 `PST_PORT` 得到的最终端口及其覆盖来源也会同步写入该数据库。以下旧配置入口已经删除，不提供兼容读取路径：

- `config.yaml`
- `-config` 命令行参数
- `WEB__*`、`RCON__*`、`REST__*`、`SAVE__*`、`TASK__*`、`MANAGE__*` 等 PST 环境变量

> [!TIP]
> `sav_cli` 默认从 PST 可执行文件所在目录自动查找；一般不需要手动填写解析工具路径。

## 开发与接口文档

- [APIFox 在线接口文档](https://q4ly3bfcop.apifox.cn/)
- 本地 Swagger：`http://127.0.0.1:8080/swagger/index.html`

本分支新增的管理员接口：

- `GET /api/server/process`
- `POST /api/server/save`
- `POST /api/server/start`
- `POST /api/server/restart`
- `POST /api/server/stop`
- `POST /api/server/watchdog`
- `GET /api/base-camps/aliases`
- `PUT /api/base-camps/:base_id/alias`
- `DELETE /api/base-camps/:base_id/alias`

本地验证：

```bash
go test ./...
cd web
pnpm install --frozen-lockfile
pnpm build
```

## 感谢

- [palworld-save-tools](https://github.com/cheahjs/palworld-save-tools) 提供存档解析工具实现
- [palworld-server-toolkit](https://github.com/magicbear/palworld-server-toolkit) 提供存档高性能解析部分实现
- [pal-conf](https://github.com/Bluefissure/pal-conf) 提供游戏服务器配置生成器
- [PalEdit](https://github.com/EternalWraith/PalEdit) 提供最初的数据化思路及逻辑
- [gorcon](https://github.com/gorcon/rcon) 提供 RCON 请求/接收基础能力

## 许可证

根据 [Apache 2.0 许可证](LICENSE) 授权，任何转载请在 README 和文件部分标明；任何商用行为请务必告知。
