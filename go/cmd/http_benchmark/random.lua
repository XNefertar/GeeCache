-- random.lua
-- 用于 wrk 压测 GeeCache

-- 初始化随机种子
init = function(args)
   math.randomseed(os.time())
end

-- 生成请求
request = function()
   -- 生成 0 到 9999 之间的随机 key
   -- 对应 main.go 中的 DBSize
   local key_id = math.random(0, 9999)
   local path = "/_geecache/benchmark/key_" .. key_id
   return wrk.format(nil, path)
end
