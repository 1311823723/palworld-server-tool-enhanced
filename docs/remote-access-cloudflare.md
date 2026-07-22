# 通过 Cloudflare Tunnel 远程访问 PST

本文用于让手机在不打开 PST、游戏 REST API 或 RCON 公网端口的情况下访问 PST 网页管理端。

## 推荐拓扑

```text
手机浏览器
    ↓ HTTPS
Cloudflare Access
    ↓ Cloudflare Tunnel
cloudflared（与 PST 同一台服务器）
    ↓ 本机回环连接
http://127.0.0.1:8080
```

Cloudflare Access 只负责保护入口，PST 管理员密码和 JWT 仍然有效。访问普通页面的用户可以查看公开快照；保存世界、广播、启动、重启、停服、库存详情、配种配置和世界设置仍需要 PST 管理员登录。

## 部署步骤

1. 在 Cloudflare 中准备一个已接入的域名，例如 `example.com`，并创建 Tunnel。
2. 在运行 PST 的 Windows 主机上安装 `cloudflared`，完成登录并绑定 Tunnel。
3. 复制 `deploy/cloudflared/config.yml.example` 为 `C:\cloudflared\config.yml`，替换 Tunnel ID、凭据文件路径和域名。
4. 确认 PST 在本机 `127.0.0.1:8080` 可访问，然后启动 Tunnel：

```powershell
cloudflared tunnel --config C:\cloudflared\config.yml run YOUR_TUNNEL_NAME
```

5. 在 Cloudflare Zero Trust → Access → Applications 中添加 `https://pst.example.com`。
6. 至少创建一个只允许自己邮箱的 Allow 策略，推荐使用一次性验证码或已有身份提供商。
7. 确认手机必须先通过 Cloudflare Access，再看到 PST 页面；进入 PST 后如需管理操作，再输入 PST 管理员密码。

## 安装为 Windows 服务

管理员 PowerShell 执行：

```powershell
cloudflared service install
```

将配置放在 `C:\Windows\System32\config\system-local` 或当前 cloudflared 服务实际读取的配置目录，并检查服务状态：

```powershell
Get-Service cloudflared
Restart-Service cloudflared
```

不同版本的 cloudflared 服务读取路径可能不同。若服务启动失败，先使用前台命令运行并查看输出，再按照安装版本的服务日志位置排查。

## 防火墙和安全检查

- 不要为 PST 的 `8080` 端口配置路由器端口转发。
- 不要把游戏 REST API `8212`、RCON `25575` 或 PST 进程控制 API 直接暴露公网。
- Cloudflare Access 应限制到明确的邮箱或身份组，不要使用“所有人允许”。
- Tunnel 凭据 JSON、`config.db`、`pst.db`、游戏存档和管理员密码不能提交到 Git。
- 如果必须保留局域网访问，允许内网访问即可；公网访问只走 Cloudflare Tunnel。
- 使用手机流量测试，不要只在 Wi-Fi 下打开域名。

## 常见问题

### 页面能打开但数据请求失败

检查浏览器开发者工具中的 `/api` 请求是否仍然使用当前域名，确认没有把前端构建产物部署到另一个域名。PST 前端使用同源 `/api`，不需要额外配置 CORS。

### 页面要求登录但 Access 已通过

这是预期行为。Cloudflare Access 保护入口，PST JWT 保护管理员接口，两者是两层独立认证。访客可以看公开快照；管理员操作需要在 PST 页面输入管理员密码。

### Tunnel 断开

检查 Windows 服务、Tunnel 状态和本机端口：

```powershell
Get-Service cloudflared
Test-NetConnection 127.0.0.1 -Port 8080
```

## Tailscale 替代方案

如果不需要公开域名，可以在 PST 主机和手机安装 Tailscale，通过 Tailscale 私网地址访问 PST。手机需要保持 Tailscale 登录，且仍建议保留 PST 管理员鉴权。该方式不需要路由器端口转发，也不需要 Cloudflare Access。
