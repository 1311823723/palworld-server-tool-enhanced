-- Minimal JSON codec for PST Production Bridge.
-- Supports the JSON types used by the fixed local IPC protocol.
local json = {}

local escapes = {
    ['"'] = '\\"', ['\\'] = '\\\\', ['\b'] = '\\b', ['\f'] = '\\f',
    ['\n'] = '\\n', ['\r'] = '\\r', ['\t'] = '\\t'
}

local function encode_string(value)
    return '"' .. value:gsub('[%z\1-\31\\"]', function(char)
        return escapes[char] or string.format("\\u%04x", char:byte())
    end) .. '"'
end

local function is_array(value)
    local count = 0
    for key in pairs(value) do
        if type(key) ~= "number" or key < 1 or key % 1 ~= 0 then return false end
        count = count + 1
    end
    return count == #value
end

local function encode(value, stack)
    local kind = type(value)
    if kind == "nil" then return "null" end
    if kind == "boolean" then return value and "true" or "false" end
    if kind == "number" then
        if value ~= value or value == math.huge or value == -math.huge then
            error("non-finite number")
        end
        return tostring(value)
    end
    if kind == "string" then return encode_string(value) end
    if kind ~= "table" then error("unsupported JSON type: " .. kind) end
    stack = stack or {}
    if stack[value] then error("circular JSON value") end
    stack[value] = true
    local parts = {}
    if is_array(value) then
        for index = 1, #value do parts[#parts + 1] = encode(value[index], stack) end
        stack[value] = nil
        return "[" .. table.concat(parts, ",") .. "]"
    end
    for key, item in pairs(value) do
        if type(key) ~= "string" then error("JSON object keys must be strings") end
        parts[#parts + 1] = encode_string(key) .. ":" .. encode(item, stack)
    end
    table.sort(parts)
    stack[value] = nil
    return "{" .. table.concat(parts, ",") .. "}"
end

function json.encode(value)
    return encode(value)
end

local function decoder(source)
    local position = 1
    local length = #source

    local function skip_space()
        while position <= length and source:sub(position, position):match("%s") do
            position = position + 1
        end
    end

    local parse_value

    local function parse_string()
        position = position + 1
        local result = {}
        while position <= length do
            local char = source:sub(position, position)
            if char == '"' then
                position = position + 1
                return table.concat(result)
            end
            if char == "\\" then
                position = position + 1
                local escape = source:sub(position, position)
                local mapped = {
                    ['"'] = '"', ['\\'] = '\\', ['/'] = '/', b = '\b',
                    f = '\f', n = '\n', r = '\r', t = '\t'
                }
                if mapped[escape] then
                    result[#result + 1] = mapped[escape]
                    position = position + 1
                elseif escape == "u" then
                    local hex = source:sub(position + 1, position + 4)
                    local code = tonumber(hex, 16)
                    if not code then error("invalid unicode escape") end
                    if code < 128 then
                        result[#result + 1] = string.char(code)
                    elseif code < 2048 then
                        result[#result + 1] = string.char(192 + math.floor(code / 64), 128 + code % 64)
                    else
                        result[#result + 1] = string.char(
                            224 + math.floor(code / 4096),
                            128 + math.floor(code / 64) % 64,
                            128 + code % 64
                        )
                    end
                    position = position + 5
                else
                    error("invalid string escape")
                end
            else
                if char:byte() < 32 then error("control character in string") end
                result[#result + 1] = char
                position = position + 1
            end
        end
        error("unterminated string")
    end

    local function parse_number()
        local start = position
        while position <= length and source:sub(position, position):match("[%d%+%-%.eE]") do
            position = position + 1
        end
        local value = tonumber(source:sub(start, position - 1))
        if value == nil then error("invalid number") end
        return value
    end

    local function parse_array()
        position = position + 1
        local result = {}
        skip_space()
        if source:sub(position, position) == "]" then
            position = position + 1
            return result
        end
        while true do
            result[#result + 1] = parse_value()
            skip_space()
            local char = source:sub(position, position)
            if char == "]" then
                position = position + 1
                return result
            end
            if char ~= "," then error("expected comma in array") end
            position = position + 1
        end
    end

    local function parse_object()
        position = position + 1
        local result = {}
        skip_space()
        if source:sub(position, position) == "}" then
            position = position + 1
            return result
        end
        while true do
            skip_space()
            if source:sub(position, position) ~= '"' then error("expected object key") end
            local key = parse_string()
            skip_space()
            if source:sub(position, position) ~= ":" then error("expected colon") end
            position = position + 1
            result[key] = parse_value()
            skip_space()
            local char = source:sub(position, position)
            if char == "}" then
                position = position + 1
                return result
            end
            if char ~= "," then error("expected comma in object") end
            position = position + 1
        end
    end

    function parse_value()
        skip_space()
        local char = source:sub(position, position)
        if char == '"' then return parse_string() end
        if char == "{" then return parse_object() end
        if char == "[" then return parse_array() end
        if char == "-" or char:match("%d") then return parse_number() end
        if source:sub(position, position + 3) == "true" then position = position + 4; return true end
        if source:sub(position, position + 4) == "false" then position = position + 5; return false end
        if source:sub(position, position + 3) == "null" then position = position + 4; return nil end
        error("invalid JSON value")
    end

    local value = parse_value()
    skip_space()
    if position <= length then error("trailing JSON data") end
    return value
end

function json.decode(source)
    if type(source) ~= "string" then error("JSON input must be a string") end
    return decoder(source)
end

return json
