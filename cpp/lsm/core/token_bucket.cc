#include "token_bucket.h"
#include <thread>

namespace lsm {
    TokenBucket::TokenBucket(double capacity, double refill_rate)
        : _tokens(capacity)
        , _capacity(capacity)
        , _refill_rate(refill_rate)
        , _last_refill_time(Clock::now()) {}

    void TokenBucket::Refill() {
        auto now = Clock::now();
        double seconds = std::chrono::duration<double>(now - _last_refill_time).count();

        if (seconds > 0) {
            double new_tokens = seconds * _refill_rate;
            _tokens = std::min(_capacity, _tokens + new_tokens);
            _last_refill_time = now;
        }
    }

    void TokenBucket::SetRefillRate(double new_rate) {
        std::lock_guard<std::mutex> lock(_mu);
        Refill();
        _refill_rate = new_rate;
    }

    double TokenBucket::GetRefillRate() const {
        std::lock_guard<std::mutex> lock(_mu);
        return _refill_rate;
    }

    bool TokenBucket::Consume(size_t bytes, int max_wait_ms) {
        if (bytes == 0) return true;

        auto start_time = Clock::now();

        while (true) {
            {
                std::lock_guard<std::mutex> lock(_mu);
                Refill();

                if (_tokens >= bytes) {
                    _tokens -= bytes;
                    return true;
                }
            }

            auto elapsed_ms = std::chrono::duration_cast<std::chrono::milliseconds>(Clock::now() - start_time).count();
            if (elapsed_ms >= max_wait_ms) {
                // 超时处理策略：
                // 1. 硬拒绝：return false;
                // 2. 软限流：如果还有剩余token，扣成负数也允许通过（允许透支），只要别太离谱。
                // 这里我们选择返回 true 但处于欠费状态（Bursty），或者严格返回 false。
                // 简单起见，返回 true 允许通过，但通过日志告警可能是更好的软限流方式。
                // 现在的实现：严格返回 false 代表被限流。
                return false; 
            }

            std::this_thread::sleep_for(std::chrono::milliseconds(10));
        }
    }
}