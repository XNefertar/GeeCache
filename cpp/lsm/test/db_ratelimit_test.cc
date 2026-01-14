#include "core/db.h"
#include <iostream>
#include <chrono>
#include <cassert>
#include <filesystem>
#include <vector>
#include <thread>
#include <iomanip>

namespace fs = std::filesystem;
using namespace lsm;

void CleanDB(const std::string& path) {
    if (fs::exists(path)) {
        fs::remove_all(path);
    }
}

long RunTest(bool enable_limit, double limit_rate, int data_mb) {
    std::string db_path = "/tmp/lsm_test_ratelimit_" + std::string(enable_limit ? "on" : "off");
    CleanDB(db_path);

    Options opts;
    if (enable_limit) {
        opts.write_rate_limit = limit_rate;
    } else {
        opts.write_rate_limit = 0.0;
    }
    
    // Disable background sync to make it pure memory speed vs rate limit
    opts.sync = false; 
    
    DB db(db_path, opts);
    
    // 0.5 MB value
    int val_size = 1024 * 512;
    std::string large_val(val_size, 'x'); 
    
    int count = (data_mb * 1024 * 1024) / val_size;
    
    std::cout << "[Test] Rate Limit: " << (enable_limit ? std::to_string((int)limit_rate/1024/1024) + " MB/s" : "Unlimited") 
              << ", Data: " << data_mb << " MB ... " << std::flush;
    
    auto start = std::chrono::steady_clock::now();
    
    for (int i = 0; i < count; ++i) {
        db.Put("key" + std::to_string(i), large_val);
    }
    
    auto end = std::chrono::steady_clock::now();
    auto duration_ms = std::chrono::duration_cast<std::chrono::milliseconds>(end - start).count();
    
    std::cout << "Done in " << duration_ms << " ms" << std::endl;
    return duration_ms;
}

int main() {
    std::cout << "=== Comparator Test: Rate Limit ON vs OFF ===" << std::endl;

    // Phase 1: Unlimited
    // Should be extremely fast as it just hits MemTable (and WAL if not sync)
    long duration_off = RunTest(false, 0, 10); // 10MB
    
    // Phase 2: Limited to 2MB/s
    // 10MB data.
    // Capacity = 2MB.
    // Instant burst: 2MB.
    // Remaining: 8MB.
    // Refill rate: 2MB/s -> 4 seconds wait.
    long duration_on = RunTest(true, 2.0 * 1024 * 1024, 10); // 10MB, 2MB/s limit
    
    std::cout << "\n=== Results ===" << std::endl;
    std::cout << "Unlimited duration : " << duration_off << " ms" << std::endl;
    std::cout << "Limited duration   : " << duration_on << " ms" << std::endl;
    
    double ratio = (double)duration_on / (double)std::max(duration_off, 1L);
    std::cout << "Slowdown ratio     : " << std::fixed << std::setprecision(2) << ratio << "x" << std::endl;

    if (duration_on < duration_off * 2 || duration_on < 3000) {
        std::cerr << "Fail: Rate limited run was too fast! Limit did not work effectively." << std::endl;
        return 1;
    }

    std::cout << "Pass: Rate limiting effectively throttled throughput." << std::endl;
    return 0;
}

