# 配种农场存档字段与能力矩阵

本文记录 PST 配种农场解析所依据的直接存档引用。实现不使用坐标距离、建筑英文名称片段、父母组合或附近工作帕鲁进行推断。

## 验证范围

解析链已使用一份未提交到 Git 的 Palworld 1.0 当前存档及其连续备份验证，样本日期为 2026-07-22。对照状态覆盖空农场、两只父母、父母更换、同性父母、蛋糕消耗、0/多枚待拾取蛋以及蛋被取走；同一存档中存在两个严格识别出的配种农场。真实玩家、UUID、路径和存档内容均未进入代码、测试或文档。

自动测试使用完全合成且脱敏的 UUID 和数据结构，不依赖真实 `Level.sav`。

Palworld 存档本身未提供可安全展示的精确补丁号，因此 `validated_game_version` 使用 `Palworld 1.0 save fixture (2026-07-22)`。若后续版本改变任一关键路径，应清空该能力标记并重新建立对照存档，而不是继续猜测。

## 能力矩阵

| 字段 | 已验证数据路径 | 可靠 | 实时 | 缺失或变化时行为 |
|---|---|---:|---|---|
| 农场识别 | `MapObjectSaveData.values[].MapObjectId == BreedFarm` 且 `ConcreteModel.RawData.concrete_model_type == PalMapObjectBreedFarmModel` | 是 | 否，存档快照 | 不返回为农场，记录诊断警告 |
| 所属据点 | `Model.RawData.base_camp_id_belong_to` → `BaseCampSaveData` | 是 | 否 | 农场关联不可靠，不产生提醒 |
| 所属公会 | `Model.RawData.group_id_belong_to` → `GroupSaveDataMap`，并与据点公会一致 | 是 | 否 | 农场关联不可靠，不产生提醒 |
| 两只父母 | `WorkSaveData.RawData.owner_map_object_model_id` → 农场 ID；`assign_define_data_id == BreedFarm_0`；`WorkAssignMap.RawData.location_index` 为 0/1；`assigned_individual_id.instance_id` → `CharacterSaveParameterMap` | 是 | 否 | 对应槽位为空；不从附近帕鲁补全 |
| 蛋糕容器 | 农场 `ConcreteModel.ModuleMap[ItemContainer].RawData.target_container_id` → `ItemContainerSaveData` | 是 | 否 | `cake_count=null`、`verified=false`，记录警告 |
| 蛋糕数量 | 专属容器槽位中经过验证的内部 ID：`Cake`、`Cake02`、`Cake03`、`Cake04`、`Cake05` | 是 | 否 | 未知物品不计入蛋糕，并记录非蛋糕物品警告 |
| 已产蛋关联 | 农场 `ConcreteModel.RawData.spawned_egg_instance_ids[]` → `MapObjectSaveData.Model.RawData.instance_id`，目标静态类型为验证过的 `Palegg` 系列 | 是 | 否 | `egg_count=null`、快照不完整、不产生提醒 |
| 已产蛋数量 | 上述稳定实例集合的数量 | 是 | 否 | 不产生提醒 |
| 蛋稳定实例 ID | `spawned_egg_instance_ids[]` 的每个 UUID | 是 | 否 | 降级为物品类型多重集合；相同数量的模糊变化不提醒 |
| 蛋种类/孵化结果 | 当前样本只显示通用 `Palegg` MapObject，未找到可靠类型字段 | 否 | 否 | `egg_item_id=null`、`egg_name=Unknown egg` |
| 配种进度 | 未验证 | 否 | 否 | `progress=null`，UI 显示不支持 |
| 正在配种/暂停/暂停原因 | 未验证 | 否 | 否 | 不声称“正在配种”；仅显示可直接推导的缺父母、蛋糕为空或有蛋状态 |
| 预计完成/产出时间 | 未验证 | 否 | 否 | 返回空值；事件时间是 PST 检测时间，不冒充游戏产出时间 |
| 农场损坏/停工 | 未验证 | 否 | 否 | 不推断，仅展示解析警告 |

## 关系验证细节

### 农场、据点与公会

农场必须同时满足精确静态 ID 和精确 ConcreteModel 类型。随后以农场模型中保存的据点 UUID 和公会 UUID 做双重引用，并验证据点所属公会一致。坐标只用于页面展示和人工诊断，不参与关联。

### 父母槽位

父母来自所有者为该农场 MapObject ID 的 `WorkSaveData`。只有定义 ID 为 `BreedFarm_0` 且位置槽为 0 或 1 的实例才会返回。性别来自角色真实 `Gender` 字段；槽位 0/1 不被解释为父方/母方，因此同性父母也会按真实值显示。

### 蛋糕

每个农场必须恰好解析到一个直接 `ItemContainer` 模块引用。PST 只读取这个 UUID 对应容器中的内部蛋糕物品 ID，不读取玩家背包、普通箱子、冰箱或其他农场容器。

### 待拾取蛋

当前版本的农场 ConcreteModel 直接保存 `spawned_egg_instance_ids`。连续备份验证了集合从 0 增长到多枚、保留原实例并增加新实例，以及玩家取走后集合归零。每个 ID 都能唯一匹配一个 `Palegg` MapObject，因此可用于事件去重。样本未提供可靠蛋种字段，PST 不根据父母组合预测蛋种。

## 降级与版本兼容

只要游戏版本未验证、关键能力为 false、农场解析不完整、任何蛋关联未验证、存档时间倒退或上一次解析失败，提醒处理器就只更新安全基线，不创建事件。解析恢复后的第一份可靠快照仍只建立基线，下一次真正新增时才提醒。

字段映射升级时应补充新的隐私安全对照存档和合成测试，再更新 `VALIDATED_GAME_VERSION`。不得仅根据字段名包含 `farm`、`egg`、`cake` 或 `breeding` 恢复能力。
