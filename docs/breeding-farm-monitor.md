# 配种农场监控与产蛋提醒

配种农场页面位于 `/breeding-farms`，仅登录 PST 管理模式后可见。后端全部接口使用现有管理员 JWT；隐藏导航不作为权限控制。

## 页面内容

页面按据点分组显示农场 ID、坐标、公会、两只真实分配槽中的帕鲁、性别/种类/昵称/等级、农场专属蛋糕数量、待拾取蛋数量、最后检测到新蛋的时间、可信度和解析警告。数据来自最近一次成功解析的服务器存档，不是游戏实时遥测。

当前版本没有可靠的配种进度、暂停原因和蛋种字段。这些值保持 `null` 或显示“未知/不支持”，不会根据游戏常识或父母组合补全。字段依据见 [配种农场存档字段与能力矩阵](./breeding-farm-save-fields.md)。

## 配置提醒

打开“提醒设置”后可以：

1. 开启配种监控。
2. 选择指定据点、指定农场，或二次确认后选择全部农场。
3. 选择是否在首次开启时提醒已经存在的蛋。默认关闭，首次只建立基线。
4. 选择每个新蛋分别提醒，或一次快照只提醒一次。
5. 设置触发提醒的最小待拾取蛋数量。
6. 开启游戏内广播、站内通知、浏览器通知，并设置历史保留天数。
7. 按需修改游戏内广播模板。支持 `{base}`（据点名）、`{new_count}`（本次新蛋数）和 `{count}`（当前待拾取总数）。

指定据点和指定农场都为空时不会监控任何农场。配置保存在 `config.db`；蛋集合基线、唯一事件键、历史与已读状态保存在现有 `pst.db`。

浏览器权限只会在管理员点击“授权浏览器通知”后申请。第一版通过每 20 秒轮询未读事件工作，只有 PST 页面保持打开时才承诺显示浏览器通知；关闭页面后没有后台 Web Push。

游戏内提醒在新事件持久化成功后，通过项目现有的 Palworld 官方 REST API `/v1/api/announce` 发送。一次快照中同一农场的多个新蛋会合并为一条广播，避免刷屏；广播失败不会回滚已经成功写入的存档快照和通知历史，PST 会记录不含密码的错误日志。该事件与发送渠道相互独立，后续可继续增加 QQ 机器人等通知适配器。

## 提醒状态机

- 首次启用：默认记录现有集合，不提醒；`notify_existing_on_enable=true` 时现有蛋只提醒一次。
- `0 → 1`：创建 `egg_ready` 事件。
- `1 → 2`：稳定实例集合中新 ID 创建一次事件。
- `2 → 0`：只更新基线，视为可能已拾取，不产生新蛋提醒。
- 数量相同但稳定实例被替换：新实例会提醒。
- 没有稳定 ID 时：使用 `egg_item_id + count` 多重集合；相同总数的类型变化视为不明确，默认不提醒。

事件键由农场 ID、蛋实例/多重集合增量、存档时间和当前数量确定，并由 `pst.db` 唯一去重。PST 重启、页面刷新或重复解析相同存档不会再次创建同一事件。

## 提醒延迟

实际链路是：游戏产蛋 → 服务器下一次正常保存 → PST 下一次同步与解析 → 原子切换有效快照 → 创建事件 → 发送游戏内广播 / 打开页面的下一次轮询。因此提醒一定可能晚于游戏内实际产蛋时间。

本功能复用现有存档同步周期，不会为了追求实时提醒每几秒调用官方 `/save`。不建议缩短游戏保存周期来模拟实时遥测，以免增加磁盘写入和解析负担。

## API

以下接口均要求管理员 JWT：

- `GET /api/breeding-farms`
- `GET /api/breeding-farms/:farm_id`
- `GET /api/breeding-farms/:farm_id/parents`
- `GET /api/breeding-farms/:farm_id/cakes`
- `GET /api/breeding-farms/:farm_id/eggs`
- `GET /api/breeding-farms/capabilities`
- `GET|PUT /api/breeding-farms/notification-config`
- `GET /api/breeding-farms/events`
- `GET /api/breeding-farms/events/unread`
- `POST /api/breeding-farms/events/:event_id/read`
- `POST /api/breeding-farms/events/read-all`

## 库存集成

农场专属蛋糕容器以 `breeding_farm_cake_box` 来源进入只读库存索引，并替换该容器可能存在的普通据点容器分类，因此不会重复统计。

当前已产蛋是独立 `Palegg` MapObject，且样本无法可靠取得对应物品类型，所以第一版不将它伪装成普通库存物品。待未来验证出可靠物品 ID 后，可用独立 `breeding_farm_output` 或 `world_output` 来源加入索引；同一实例仍必须全局去重。

## 安全与隐私

API 和通知不会返回或展示管理员密码、游戏密码、JWT、玩家 IP、SteamID、Cloudflare Token 或完整存档路径。真实验证存档不得提交到 Git。不要把 PST 管理接口直接暴露到公网；通过 Cloudflare Tunnel 使用时还应启用 Cloudflare Access 等额外身份验证。
