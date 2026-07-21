<h1 align='center'>幻兽帕鲁服务器管理工具</h1>

<p align="center">
   <strong>简体中文</strong> | <a href="/docs/README.en.md">English</a> | <a href="/docs/README.ja.md">日本語</a>
</p>

<p align='center'>
  通过可视化界面及 REST 接口管理幻兽帕鲁专用服务器，基于 SAV 存档解析、官方 REST API 与 RCON 实现。
</p>

<p align='center'>
<img alt="GitHub Repo stars" src="https://img.shields.io/github/stars/zaigie/palworld-server-tool?style=for-the-badge">&nbsp;&nbsp;
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
- 管理模式中的可视化 PST 配置

业务数据保存在 `pst.db`，PST 配置和管理员凭据单独保存在 `config.db`。清理或重置配置不会影响玩家、公会、RCON 和备份记录。

## 本分支新增：Windows 本地进程管理

本分支在保留原项目玩家、公会、RCON、备份、定时任务和配置中心能力的基础上，增加 Windows 本地 Palworld Dedicated Server 进程管理：

- 从 PST 管理界面启动固定路径的 `PalServer.exe`
- 通过官方 REST API 保存世界、平滑停服和平滑重启
- 服务器意外退出后延迟自动重启，并在短时间连续失败时触发崩溃循环保护
- 管理员手动停服后保持关闭，不会被守护程序再次拉起
- 在桌面端和移动端查看 PID、运行时间、守护状态、最近退出和错误
- PST 重启时识别已经运行的 `PalServer.exe` 或 `PalServer-Win64-Shipping-Cmd.exe`，避免重复启动

### 配置 Windows PalServer

进入管理模式，打开“PST 配置”中的“Windows PalServer 进程”，填写：

1. 开启“本机进程管理”。
2. 将“PalServer.exe 路径”设为服务器实际路径，例如 `D:\Program Files\Steam\steamapps\common\PalServer\PalServer.exe`。
3. 工作目录可以留空；留空时使用可执行文件所在目录。
4. 每条启动参数单独添加，例如 `-port=8211`、`-players=8`、`-logformat=text`。不要输入一整条 CMD 命令。
5. 按需开启崩溃守护，并配置重启等待时间、连续失败上限和统计窗口。

保存后 supervisor 会立即加载新配置，但不会擅自重启当前正在运行的游戏服务器。回到管理员概览的“服务器进程”卡片即可保存世界、启动、平滑重启、平滑停服或切换守护状态。重启会等待旧进程实际退出，再从退出时刻开始计算重启延迟，不会同时启动两个实例。

Windows 是本功能的首要支持平台。Linux 和 macOS 仍可使用 PST 原有管理功能，但尝试启动本地 PalServer 进程时会收到明确的 `unsupported platform` 错误。Docker 部署也不应使用宿主机 Windows 进程管理，应继续通过 REST API、RCON 或 `pst-agent` 管理独立游戏服务器。

### 进程控制安全注意事项

- 进程控制接口全部要求 PST 管理员 JWT，不提供匿名访问。
- PST 只允许启动配置中的 `PalServer.exe` 或 `PalServer-Win64-Shipping-Cmd.exe`，不提供任意 CMD、PowerShell 或 Shell 执行接口。
- 配置中的启动参数是字符串数组；`&`、`|`、`>`、`<`、`cmd.exe` 和 `powershell.exe` 会被后端拒绝。
- 配置查询不会返回 RCON 密码、REST API 密码、JWT 或其他敏感凭据。示例密码统一使用 `YOUR_ADMIN_PASSWORD`。
- 不要把游戏 REST API 端口 `8212`、RCON 端口 `25575` 或 PST 进程控制 API 直接暴露到公网。
- 通过 Cloudflare Tunnel 发布 PST 时，除 PST 登录外必须再启用 Cloudflare Access 等额外身份验证，并限制可访问用户。

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

1. 从 [GitHub Releases](https://github.com/zaigie/palworld-server-tool/releases) 下载对应系统和架构的压缩包并解压。
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
