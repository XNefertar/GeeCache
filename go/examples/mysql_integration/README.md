# MySQL + GeeCache 集成指南

本目录包含了一个完整的代码示例 (`main.go`)，展示了如何将 GeeCache 作为 MySQL 的缓存层使用。

## 核心集成逻辑 (Read-Through 模式)

GeeCache 采用 **Read-Through** 模式，这意味着你不需要在业务代码里写 `if cache.Get() == nil { db.query(); cache.Set() }` 这样繁琐的代码。

你需要做的仅仅是： **告诉 GeeCache，如果缓存没命中，去哪里取数据。**

### 1. 架构图解

```plaintext
Client -> [ GetUser() ] -> GeeCache .Get(key) 
                              |
                    Hit? <----+----> Yes -> Return Cached Data
                              |
                              No
                              |
                    [ GetterFunc ]  <-- 你的集成代码在这里
                              |
                        Query MySQL
                              |
                    Serialize (JSON)
                              |
                    Store in Cache & Return
```

### 2. 你需要编写的三部分代码

1.  **Getter 定义**：在 `geecache.NewGroup` 时，传入一个查询 MySQL 的匿名函数。
2.  **序列化**：数据库返回的是 struct，缓存存的是 `[]byte`。常用的方案是 JSON (`json.Marshal`) 或 Protocol Buffers。
3.  **Wrapper 方法**：在业务层封装一个 `GetUser` 方法，内部调用 `geecache.Get` 并反序列化回 Struct。

详细实现请参考同目录下的 [main.go](main.go)。
