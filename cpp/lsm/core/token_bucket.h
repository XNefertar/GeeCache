#pragma once
#include <chrono>
#include <mutex>
#include <atomic>
#include <algorithm>

namespace lsm {
    class TokenBucket {
    public:
        // capacity: 桶容量（最大突发字节数）
        // refill_rate: 初始填充速率（字节/秒）
        TokenBucket(double capacity, double refill_rate);
        ~TokenBucket() = default;

        // 尝试消耗 tokens。
        // 如果桶中不够，会最多阻塞 max_wait_ms 毫秒。
        // 返回 true 表示成功拿到令牌（或部分拿到但允许通过），false 表示超时/限流拒绝。
        bool Consume(size_t bytes, int max_wait_ms = 100);

        // 动态调整填充速率（线程安全）
        void SetRefillRate(double new_rate);

        // 获取当前速率（用于监控）
        double GetRefillRate() const;

    private:
        void Refill();

        mutable std::mutex _mu;
        double _tokens;           // 当前可用令牌数
        double _capacity;         // 桶上限
        double _refill_rate;      // 每秒补充的令牌数

        using Clock = std::chrono::steady_clock;
        Clock::time_point _last_refill_time;
    };
}