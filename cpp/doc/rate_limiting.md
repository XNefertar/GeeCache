# Token Bucket Rate Limiter Implementation & Testing | 令牌桶限流器实现与测试
`2026-01-14`

## 1. Overview / 概述
This document details the implementation of a Token Bucket rate limiter integrated into the LSM-Tree storage engine (`lsm::DB`) to provide Quality of Service (QoS) controls. The primary goal is to prevent "Write Stalls" and ensure read latency stability under heavy write loads.

本文档详细介绍了集成到 LSM-Tree 存储引擎 (`lsm::DB`) 中的令牌桶限流器的实现，旨在提供服务质量 (QoS) 控制。其主要目标是防止“写停顿 (Write Stalls)”并在高写入负载下确保读取延迟的稳定性。

## 2. Architecture / 架构

### 2.1 The Token Bucket Algorithm / 令牌桶算法
Implemented in `cpp/lsm/core/token_bucket.h` and `cpp/lsm/core/token_bucket.cc`.
实现位置：`cpp/lsm/core/token_bucket.h` 与 `cpp/lsm/core/token_bucket.cc`。

- **Mechanism**: Tokens are refilled at a fixed rate (`bytes/sec`). A write operation must consume tokens equal to its data size before proceeding.
- **机制**：令牌按固定速率 (`bytes/sec`) 填充。写入操作必须在执行前消耗与其数据大小相等的令牌。
- **Components / 组件**:
  - `capacity`: Maximum burst size (bytes). 最大突发大小（字节）。
  - `refill_rate`: Steady-state throughput limit (bytes/sec). 稳态吞吐量限制（字节/秒）。
  - `Consume(size)`: Blocks the caller until enough tokens are available. 阻塞调用者直到有足够的令牌可用。

### 2.2 Integration Point / 集成点
The rate limiter operates at the entry point of the `DB::Put` method in `cpp/lsm/core/db.cc`.
限流器在 `cpp/lsm/core/db.cc` 中 `DB::Put` 方法的入口处运行。

```cpp
void DB::Put(const std::string& key, const std::string& value) {
    if (_rate_limiter) {
        // Chunked consumption to avoid starvation for large writes
        // 分块消耗，避免大写入导致的饥饿问题
        size_t remaining = key.size() + value.size();
        while (remaining > 0) {
            size_t chunk = std::min(remaining, kChunkSize);
            while (!_rate_limiter->Consume(chunk, 1000 /* timeout_ms */)) { ... }
            remaining -= chunk;
        }
    }
    // ... proceed to acquire mutex and write to MemTable ...
    // ... 继续获取互斥锁并写入 MemTable ...
}
```

**Key Benefit**: By throttling *before* acquiring the global DB mutex (`_mutex`), we prevent write-heavy workloads from monopolizing the lock, allowing readers (`DB::Get`) to interleave successfully.

**核心优势**：通过在获取全局数据库互斥锁 (`_mutex`) *之前* 进行节流，我们防止了高写入负载长期独占锁，从而允许读取者 (`DB::Get`) 成功插入执行。

## 3. Configuration / 配置
Enabled via `Options` passed to `DB::DB()`:
通过传递给 `DB::DB()` 的 `Options` 启用：
```cpp
struct Options {
    // ...
    double write_rate_limit = 0.0; // 0.0 = Unlimited (无限制)
};
```
If `write_rate_limit > 0`, a `TokenBucket` is initialized with:
如果 `write_rate_limit > 0`，`TokenBucket` 将被初始化：
- `Refill Rate`: Equal to `write_rate_limit`. 填充率等于 `write_rate_limit`。
- `Capacity`: `std::max(rate, 4096.0)` (to handle minimum IO block sizes). 容量设置为速率与 4096.0 的较大值（以处理最小 IO 块大小）。

## 4. Testing & Validation / 测试与验证

### 4.1 Methodology / 方法论
We use a **Mixed Workload Test** (`cpp/lsm/test/db_ratelimit_test.cc`) to measure the impact of rate limiting on read latency ("QoS").
我们使用 **混合负载测试** (`cpp/lsm/test/db_ratelimit_test.cc`) 来测量限流对读取延迟（即“QoS”）的影响。

- **Scenario / 场景**: 
  - **Writer**: Writes 5MB of data as fast as possible.
  - **Writer**: 尽可能快地写入 5MB 数据。
  - **Reader**: Simultaneously performs random reads in a background thread.
  - **Reader**: 同时在后台线程中执行随机读取。
  - **Environment**: `sync=false` (Memory-speed writes) to maximize CPU/Lock contention.
  - **Environment**: `sync=false`（内存级写入速度），以最大化 CPU/锁争用。

### 4.2 Benchmark Results (Local) / 基准测试结果（本地）

**Configuration / 配置**:
- Data Size: 5 MB
- Rate Limit: 2 MB/s
- Sync: `false` (to simulate high contention / 模拟高争用)

| Metric (指标) | Unlimited / Spike (无限制/突发) | Limited / Smooth (有限制/平滑) | Improvement (提升) |
| :--- | :--- | :--- | :--- |
| **Write Time (写入时间)** | **11 ms** | **2505 ms** | N/A (Intentionally slower / 有意变慢) |
| **Avg Read Latency (平均读延迟)** | **821.88 us** | **18.39 us** | **44.7x Faster (快 44.7 倍)** |
| **P99 Read Latency (P99 读延迟)** | **5113.00 us** | **44.00 us** | **116x More Stable (稳定 116 倍)** |

### 4.3 Analysis / 分析
1.  **Unlimited Case (无限制)**: The writer floods the MemTable, holding the `_mutex` almost continuously. The reader thread is starved, leading to high (>1.5ms) and unpredictable (>6ms P99) latency.
    **无限制场景**：写入者瞬间淹没 MemTable，几乎持续占用 `_mutex`。读取线程被饿死，导致极高（>1.5ms）且不可预测（>6ms P99）的延迟。
2.  **Limited Case (有限制)**: The writer is forced to sleep frequently to wait for tokens. These sleep intervals release the `_mutex`, giving the reader thread ample opportunity to acquire the lock and serve requests immediately.
    **有限制场景**：写入者被迫频繁休眠以等待令牌。这些休眠间隔释放了 `_mutex`，给予读取线程充足的机会获取锁并立即服务请求。

**Conclusion**: Rate limiting successfully trades raw write throughput for system stability and read responsiveness (QoS).
**结论**：限流成功地以牺牲原始写入吞吐量为代价，换取了系统稳定性和读取响应速度 (QoS)。

## 5. Usage / 用法
To reproduce the test results:
复现测试结果：
```bash
cd cpp/lsm/build
make db_ratelimit_test
./db_ratelimit_test
```
