local json = require("json")

local BRIDGE_VERSION = "0.1.1"
local PROTOCOL_VERSION = 1
local MOD_NAME = "PSTProductionBridge"

local function with_trailing_separator(path)
    if not path or path == "" then return nil end
    local last = path:sub(-1)
    if last == "\\" or last == "/" then return path end
    return path .. "\\"
end

local function find_saved_directory(node, depth, visited)
    if depth > 6 or type(node) ~= "table" or visited[node] then return nil end
    visited[node] = true

    local ok_name, name = pcall(function() return node.__name end)
    local ok_path, absolute_path = pcall(function() return node.__absolute_path end)
    if ok_name and ok_path and type(name) == "string" and type(absolute_path) == "string" then
        local normalized = absolute_path:gsub("/", "\\")
        if name:lower() == "saved" and normalized:lower():match("\\pal\\saved$") then
            return normalized
        end
    end

    local ok_pairs, result = pcall(function()
        for key, child in pairs(node) do
            if type(key) ~= "string" or key:sub(1, 2) ~= "__" then
                local found = find_saved_directory(child, depth + 1, visited)
                if found then return found end
            end
        end
        return nil
    end)
    if ok_pairs then return result end
    return nil
end

local function resolve_ipc_root()
    -- PST injects this path into the supervised PalServer process. It is
    -- independent of WorkshopRootDir and UE4SS's current working directory.
    local configured = os.getenv("PST_BRIDGE_IPC_ROOT")
    if configured and configured ~= "" then
        return with_trailing_separator(configured)
    end

    -- Manual/external starts do not inherit the PST environment. UE4SS exposes
    -- the game directory tree, so locate Pal\Saved without assuming a CWD.
    local ok, directories = pcall(IterateGameDirectories)
    if ok then
        local saved = find_saved_directory(directories, 0, {})
        if saved then
            return with_trailing_separator(saved .. "\\" .. MOD_NAME)
        end
    end

    -- PalServer normally starts with its installation root as the CWD.
    return "Pal\\Saved\\" .. MOD_NAME .. "\\"
end

local IPC_ROOT = resolve_ipc_root()
local REQUEST_ROOT = IPC_ROOT .. "requests\\"
local RESULT_ROOT = IPC_ROOT .. "results\\"
local STATE_PATH = IPC_ROOT .. "state.json"
local KEY_PATH = IPC_ROOT .. "bridge.key"
local INDEX_PATH = REQUEST_ROOT .. "index.json"
local ACTIVE_PATH = IPC_ROOT .. "active_orders.json"
local PROCESSED_PATH = IPC_ROOT .. "processed_orders.json"
local INSTANCE_ID = tostring(os.time()) .. "-" .. tostring(math.random(100000, 999999))

local active_orders = {}
local station_locks = {}
local processed_orders = {}
local cached_catalog = {}
local adapter_reason = ""

local function log(message)
    print(string.format("[%s] %s\n", MOD_NAME, tostring(message)))
end

local function read_all(path)
    local file = io.open(path, "rb")
    if not file then return nil end
    local data = file:read("*all")
    file:close()
    return data
end

local function write_json(path, value)
    local ok, encoded = pcall(json.encode, value)
    if not ok then
        log("JSON encode failed: " .. tostring(encoded))
        return false
    end
    local temporary = path .. ".tmp"
    local file = io.open(temporary, "wb")
    if not file then return false end
    file:write(encoded)
    file:flush()
    file:close()
    os.remove(path)
    return os.rename(temporary, path) ~= nil
end

local function read_json(path)
    local data = read_all(path)
    if not data or data == "" then return nil end
    local ok, value = pcall(json.decode, data)
    if not ok then
        log("JSON decode failed for " .. path .. ": " .. tostring(value))
        return nil
    end
    return value
end

local function valid(object)
    if object == nil then return false end
    local ok, result = pcall(function() return object:IsValid() end)
    return ok and result
end

local function unreal_string(value)
    if value == nil then return "" end
    local ok, result = pcall(function() return value:ToString() end)
    if ok and result then return tostring(result) end
    return tostring(value)
end

local function safe_call(fallback, callback)
    local ok, value = pcall(callback)
    if ok and value ~= nil then return value end
    return fallback
end

local function find_station(actor_guid)
    local models = FindAllOf("PalMapObjectConvertItemModel") or {}
    for _, model in ipairs(models) do
        if valid(model) then
            local guid = safe_call("", function() return unreal_string(model:GetInstanceId()) end)
            if guid == actor_guid then return model end
        end
    end
    return nil
end

local function recipe_allowed(model, recipe_id)
    local recipes = safe_call({}, function() return model:GetRecipes() end)
    for index = 1, #recipes do
        if unreal_string(recipes[index]) == recipe_id then return true end
    end
    return false
end

local function recipe_row(recipe_id)
    local access = FindFirstOf("PalMasterDataTableAccess_ItemRecipe")
    if not valid(access) then return nil end
    return safe_call(nil, function() return access.DataTable:FindRow(FName(recipe_id)) end)
end

local function material_list(model, row)
    local materials = {}
    for index = 1, 5 do
        local id = unreal_string(row["Material" .. index .. "_Id"])
        local count = tonumber(row["Material" .. index .. "_Count"]) or 0
        if id ~= "" and id ~= "None" and count > 0 then
            local available = safe_call(0, function()
                local slot = model:GetSlotInjectableMaterial(FName(id))
                if valid(slot) then return tonumber(slot:GetStackCount()) or 0 end
                return 0
            end)
            materials[#materials + 1] = {
                item_id = id,
                required_each = count,
                available = available
            }
        end
    end
    return materials
end

local function create_ui_model(model)
    local class = StaticFindObject("/Script/Pal.PalUIConvertItemModel")
    if not class then return nil, "找不到 PalUIConvertItemModel 类" end
    local ok, ui = pcall(function()
        local object = StaticConstructObject(class, model)
        object:Initialize(model)
        return object
    end)
    if not ok or not valid(ui) or not valid(ui.ProductSettingModel) then
        return nil, "无法初始化服务器生产模型"
    end
    return ui, nil
end

local function calculate_max(model, recipe_id)
    local ui = create_ui_model(model)
    if not ui then return -1 end
    return safe_call(-1, function()
        ui.ProductSettingModel:SelectRecipe(FName(recipe_id))
        return tonumber(ui.ProductSettingModel:CalcMaxProductableNum()) or -1
    end)
end

local function scan_catalog()
    local grouped = {}
    local models = FindAllOf("PalMapObjectConvertItemModel") or {}
    for _, model in ipairs(models) do
        if valid(model) then
            local base_model = safe_call(nil, function() return model:GetBaseCampModelBelongTo() end)
            local base_id = valid(base_model)
                and safe_call("", function() return unreal_string(base_model:GetId()) end)
                or ""
            if base_id ~= "" then
                local base = grouped[base_id]
                if not base then
                    base = {
                        base_id = base_id,
                        base_name = safe_call("", function() return tostring(base_model:GetBaseCampName()) end),
                        workstations = {}
                    }
                    grouped[base_id] = base
                end
                local actor_guid = safe_call("", function() return unreal_string(model:GetInstanceId()) end)
                local station = {
                    actor_guid = actor_guid,
                    name = safe_call("", function() return unreal_string(model:TryGetMapObjectId()) end),
                    busy = (tonumber(safe_call(0, function() return model.RemainProductNum end)) or 0) > 0,
                    recipes = {}
                }
                local recipes = safe_call({}, function() return model:GetRecipes() end)
                for index = 1, #recipes do
                    local recipe_id = unreal_string(recipes[index])
                    local row = recipe_row(recipe_id)
                    if row then
                        station.recipes[#station.recipes + 1] = {
                            id = recipe_id,
                            product_item_id = unreal_string(row.Product_Id),
                            product_count = tonumber(row.Product_Count) or 1,
                            unlocked = true,
                            max_available = calculate_max(model, recipe_id),
                            materials = material_list(model, row)
                        }
                    end
                end
                base.workstations[#base.workstations + 1] = station
            end
        end
    end
    local bases = {}
    for _, base in pairs(grouped) do bases[#bases + 1] = base end
    cached_catalog = bases
end

local function save_processed_orders()
    local count = 0
    for _ in pairs(processed_orders) do count = count + 1 end
    while count > 5000 do
        local oldest_id = nil
        local oldest_time = nil
        for id, payload in pairs(processed_orders) do
            local updated = tostring(payload.updated_at or "")
            if oldest_time == nil or updated < oldest_time then
                oldest_id = id
                oldest_time = updated
            end
        end
        if oldest_id == nil then break end
        processed_orders[oldest_id] = nil
        count = count - 1
    end
    write_json(PROCESSED_PATH, processed_orders)
end

local function result(order_id, status, accepted, completed, message)
    local payload = {
        order_id = order_id,
        status = status,
        accepted_quantity = accepted or 0,
        completed_quantity = completed or 0,
        error = message or "",
        bridge_instance_id = INSTANCE_ID,
        updated_at = os.date("!%Y-%m-%dT%H:%M:%SZ")
    }
    write_json(RESULT_ROOT .. order_id .. ".json", payload)
    if status ~= "waiting_materials" then
        processed_orders[order_id] = payload
        save_processed_orders()
    end
end

local function save_active_orders()
    write_json(ACTIVE_PATH, active_orders)
end

local function validate_request(request)
    if type(request) ~= "table" then return false, "请求不是 JSON 对象" end
    if type(request.order_id) ~= "string" or not request.order_id:match("^[0-9a-fA-F%-]+$") then
        return false, "订单 ID 无效"
    end
    if request.action ~= "create" and request.action ~= "cancel" then return false, "动作无效" end
    local key = read_all(KEY_PATH)
    if not key or request.key ~= key:gsub("%s+$", "") then return false, "本地密钥无效" end
    if request.action == "cancel" then return true end
    for _, field in ipairs({"base_id", "workstation_actor_guid", "recipe_id", "quantity_mode"}) do
        if type(request[field]) ~= "string" or request[field] == "" then
            return false, field .. " 无效"
        end
    end
    if request.quantity_mode ~= "exact" and request.quantity_mode ~= "max_available" then
        return false, "数量模式无效"
    end
    if type(request.quantity) ~= "number" or request.quantity < 1 or request.quantity > 999999 then
        return false, "数量无效"
    end
    return true
end

local function dispatch_request(request)
    local ok, reason = validate_request(request)
    if not ok then
        result(request and request.order_id or "invalid", "failed", 0, 0, reason)
        return
    end
    local order_id = request.order_id
    if request.action == "cancel" then
        local active = active_orders[order_id]
        if active then
            result(order_id, "accepted", active.quantity, active.completed or 0, "游戏已经接受该订单，不能强制取消")
        else
            result(order_id, "cancelled", 0, 0, "")
        end
        return
    end
    if station_locks[request.workstation_actor_guid] then
        result(order_id, "waiting_materials", 0, 0, "该工作台已有 PST 订单正在生产")
        return
    end
    local model = find_station(request.workstation_actor_guid)
    if not valid(model) then
        result(order_id, "failed", 0, 0, "工作台已不存在")
        return
    end
    local base_id = safe_call("", function() return unreal_string(model:GetBaseCampIdBelongTo()) end)
    if base_id ~= request.base_id then
        result(order_id, "failed", 0, 0, "工作台已不属于所选据点")
        return
    end
    if not recipe_allowed(model, request.recipe_id) then
        result(order_id, "failed", 0, 0, "该工作台不支持所选配方")
        return
    end
    local remain_before = tonumber(safe_call(0, function() return model.RemainProductNum end)) or 0
    if remain_before > 0 then
        result(order_id, "waiting_materials", 0, 0, "该工作台正在执行游戏内生产任务")
        return
    end
    local ui, ui_error = create_ui_model(model)
    if not ui then
        result(order_id, "failed", 0, 0, ui_error)
        return
    end
    local accepted = request.quantity
    local started, start_error = pcall(function()
        ui.ProductSettingModel:SelectRecipe(FName(request.recipe_id))
        local maximum = tonumber(ui.ProductSettingModel:CalcMaxProductableNum()) or 0
        if request.quantity_mode == "max_available" then accepted = maximum end
        if accepted < 1 or accepted > maximum then error("当前据点材料不足") end
        ui.ProductSettingModel:SetProductNumByInput(accepted)
        ui:StartProduction()
    end)
    if not started then
        result(order_id, "failed", 0, 0, tostring(start_error))
        return
    end
    local actual_recipe = safe_call("", function() return unreal_string(model:GetCurrentRecipeId()) end)
    local actual_count = tonumber(safe_call(0, function() return model.RequestedProductNum end)) or 0
    if actual_recipe ~= request.recipe_id or actual_count < accepted then
        result(order_id, "failed", 0, 0, "Palworld 未接受生产请求")
        return
    end
    active_orders[order_id] = {
        actor_guid = request.workstation_actor_guid,
        recipe_id = request.recipe_id,
        quantity = accepted,
        completed = 0,
        last_status = "accepted",
        restored = false
    }
    station_locks[request.workstation_actor_guid] = order_id
    save_active_orders()
    result(order_id, "accepted", accepted, 0, "")
end

local function update_active_orders()
    for order_id, order in pairs(active_orders) do
        local model = find_station(order.actor_guid)
        if not valid(model) then
            result(order_id, "unknown", order.quantity, order.completed, "工作台在运行时消失")
            station_locks[order.actor_guid] = nil
            active_orders[order_id] = nil
            save_active_orders()
        else
            local remain = tonumber(safe_call(0, function() return model.RemainProductNum end)) or 0
            local completed = math.max(0, order.quantity - remain)
            local actual_recipe = safe_call("", function() return unreal_string(model:GetCurrentRecipeId()) end)
            if order.restored and (remain <= 0 or actual_recipe ~= order.recipe_id) then
                result(order_id, "unknown", order.quantity, order.completed, "Bridge 重启后无法可靠确认原工作台队列")
                station_locks[order.actor_guid] = nil
                active_orders[order_id] = nil
                save_active_orders()
            elseif remain <= 0 then
                result(order_id, "completed", order.quantity, order.quantity, "")
                station_locks[order.actor_guid] = nil
                active_orders[order_id] = nil
                save_active_orders()
            elseif order.last_status ~= "producing" or completed ~= order.completed then
                order.restored = false
                order.completed = completed
                order.last_status = "producing"
                save_active_orders()
                result(order_id, "producing", order.quantity, completed, "")
            end
        end
    end
end

local function process_requests()
    local ids = read_json(INDEX_PATH)
    if type(ids) ~= "table" then return end
    for _, order_id in ipairs(ids) do
        if type(order_id) == "string" and order_id:match("^[0-9a-fA-F%-]+$") then
            local existing = read_all(RESULT_ROOT .. order_id .. ".json")
            if not existing then
                if processed_orders[order_id] then
                    write_json(RESULT_ROOT .. order_id .. ".json", processed_orders[order_id])
                else
                    local request = read_json(REQUEST_ROOT .. order_id .. ".json")
                    if request then dispatch_request(request) end
                end
            end
        end
    end
end

local function probe_adapter()
    local class = StaticFindObject("/Script/Pal.PalUIConvertItemModel")
    if not class then
        adapter_reason = "当前 Palworld Build 缺少 PalUIConvertItemModel"
        return false
    end
    return true
end

local adapter_available = probe_adapter()

local restored_orders = read_json(ACTIVE_PATH)
local restored_processed = read_json(PROCESSED_PATH)
if type(restored_processed) == "table" then
    processed_orders = restored_processed
end
if type(restored_orders) == "table" then
    active_orders = restored_orders
    for order_id, order in pairs(active_orders) do
        if type(order) == "table" and type(order.actor_guid) == "string" then
            order.restored = true
            station_locks[order.actor_guid] = order_id
        else
            active_orders[order_id] = nil
        end
    end
end

local function publish_state()
    local state = {
        instance_id = INSTANCE_ID,
        bridge_version = BRIDGE_VERSION,
        protocol_version = PROTOCOL_VERSION,
        heartbeat_at = os.date("!%Y-%m-%dT%H:%M:%SZ"),
        compatible = adapter_available,
        reason = adapter_reason,
        capabilities = {
            catalog = true,
            orders = adapter_available,
            cancel = adapter_available
        },
        bases = cached_catalog
    }
    write_json(STATE_PATH, state)
end

ExecuteInGameThread(function()
    local ok, error_message = pcall(scan_catalog)
    if not ok then log("initial catalog scan failed: " .. tostring(error_message)) end
    publish_state()
end)

LoopAsync(500, function()
    ExecuteInGameThread(function()
        local ok, error_message = pcall(function()
            process_requests()
            update_active_orders()
        end)
        if not ok then log("request loop failed: " .. tostring(error_message)) end
    end)
    return false
end)

LoopAsync(5000, function()
    ExecuteInGameThread(function()
        local ok, error_message = pcall(scan_catalog)
        if not ok then log("catalog scan failed: " .. tostring(error_message)) end
        publish_state()
    end)
    return false
end)

log("Bridge loaded, protocol " .. tostring(PROTOCOL_VERSION) .. ", IPC " .. IPC_ROOT)
