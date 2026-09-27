local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])

local data = redis.call("HMGET", key, "tokens", "last_updated")
local tokens = tonumber(data[1])
local last_updated = tonumber(data[2])

if not tokens then
    tokens = capacity
    last_updated = now
else
    local delta = math.max(0, now - last_updated)
    local tokens_to_add = delta * refill_rate
    tokens = math.min(capacity, tokens + tokens_to_add)
    last_updated = now
end

if tokens >= requested then
    tokens = tokens - requested

    redis.call("HSET", key,
        "tokens", tokens,
        "last_updated", last_updated
    )

    redis.call("EXPIRE", key, 60)

    return {1, tokens}
else
    redis.call("HSET", key,
        "tokens", tokens,
        "last_updated", last_updated
    )

    return {0, tokens}
end