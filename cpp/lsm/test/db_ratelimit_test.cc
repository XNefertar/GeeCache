#include "core/db.h"
#include <iostream>
#include <chrono>
#include <cassert>
#include <filesystem>
#include <vector>
#include <thread>
#include <iomanip>
#include <atomic>
#include <future>
#include <numeric>
#include <mutex>
#include <algorithm>
#include <sstream>

namespace fs = std::filesystem;
using namespace lsm;

void CleanDB(const std::string& path) {
    if (fs::exists(path)) {
        fs::remove_all(path);
    }
}

struct AutoCleaner {
    std::string path;
    AutoCleaner(const std::string& p) : path(p) {}
    ~AutoCleaner() {
        CleanDB(path);
    }
};

// Result struct to hold test metrics
struct TestResult {
    long write_duration_ms;
    double avg_read_latency_us;
    double p99_read_latency_us;
};

TestResult RunMixedLoadTest(bool enable_limit, double limit_rate, int write_mb, bool use_sync) {
    std::string test_name = enable_limit ? "Limited" : "Unlimited";
    std::string db_path = "/tmp/lsm_test_mixed_" + std::string(enable_limit ? "on" : "off");
    CleanDB(db_path);

    AutoCleaner cleaner(db_path);

    Options opts;

    // 控制 sync 开关以比较内存速率（sync=false）下的效果
    opts.sync = use_sync;
    
    if (enable_limit) {
        opts.write_rate_limit = limit_rate;
    } else {
        opts.write_rate_limit = 0.0;
    }
    
    DB db(db_path, opts);
    
    // Prepare data
    int val_size = 4096; // 4KB values (typical for DBs)
    std::string val(val_size, 'x');
    int write_count = (write_mb * 1024 * 1024) / val_size;
    
    // Populate some initial data for reading
    int initial_keys = 1000;
    for(int i=0; i<initial_keys; ++i) {
         db.Put("key" + std::to_string(i), val);
    }

    std::cout << "\n[" << test_name << "] Starting (Writes: " << write_mb << "MB, Rate: " 
              << (enable_limit ? std::to_string((int)limit_rate/1024/1024) + "MB/s" : "Max") << ")..." << std::endl;

    std::atomic<bool> stop_reads{false};
    std::vector<double> read_latencies;
    std::mutex latency_mutex;

    // --- Reader Thread ---
    // Simulates user queries requiring low latency (QoS)
    auto reader_future = std::async(std::launch::async, [&]() {
        int key_idx = 0;
        int successful_reads = 0;
        while (!stop_reads) {
            auto t0 = std::chrono::steady_clock::now();
            
            std::string value;
            db.Get("key" + std::to_string(key_idx % initial_keys), &value);
            
            auto t1 = std::chrono::steady_clock::now();
            double latency_us = std::chrono::duration_cast<std::chrono::microseconds>(t1 - t0).count();
            
            {
                std::lock_guard<std::mutex> lock(latency_mutex);
                read_latencies.push_back(latency_us);
            }
            
            key_idx++;
            successful_reads++;
            // Emulate read traffic: 2000 QPS target
            std::this_thread::sleep_for(std::chrono::microseconds(500)); 
        }
        return successful_reads;
    });

    // --- Writer Thread (Main) ---
    // Simulates massive batch ingest or traffic spike
    auto write_start = std::chrono::steady_clock::now();
    
    for (int i = 0; i < write_count; ++i) {
        db.Put("new_key_" + std::to_string(i), val);
        if (write_count > 10 && i % (write_count / 10) == 0) std::cout << "." << std::flush;
    }
    
    auto write_end = std::chrono::steady_clock::now();
    long write_duration = std::chrono::duration_cast<std::chrono::milliseconds>(write_end - write_start).count();
    
    // Stop reader
    stop_reads = true;
    int reads = reader_future.get();
    std::cout << " Done. (Wrote " << write_count << " keys, Read " << reads << " times)" << std::endl;

    // Calculate Latency Stats
    double sum = 0;
    std::vector<double> sorted_latencies = read_latencies;
    std::sort(sorted_latencies.begin(), sorted_latencies.end());
    
    for (auto v : sorted_latencies) sum += v;
    double avg = sorted_latencies.empty() ? 0 : sum / sorted_latencies.size();
    size_t p99_idx = sorted_latencies.empty() ? 0 : std::min(size_t((sorted_latencies.size() - 1) * 0.99), sorted_latencies.size() - 1);
    double p99 = sorted_latencies.empty() ? 0 : sorted_latencies[p99_idx];

    return {write_duration, avg, p99};
}

int main() {
    std::cout << "=== Mixed Workload QoS Test: Latency under Load ===" << std::endl;
    std::cout << "Scenario: 2MB/s Write Limit vs Unlimited. Background Reader (QoS target). (sync=false to expose mem-speed writer)" << std::endl;

    const int write_mb = 5;
    const double limit_rate = 2.0 * 1024 * 1024;

    // 1. Unlimited Run
    // Using 5MB to avoid crushing the helper/test env with too much load causing accidents
    TestResult result_unlimited = RunMixedLoadTest(false, 0, write_mb, false); // sync=false -> memory-speed writer

    // 2. Limited Run (2MB/s)
    // 5MB at 2MB/s should take ~2.5s + sync overhead
    TestResult result_limited = RunMixedLoadTest(true, limit_rate, write_mb, false); // sync=false

    std::cout << "\n=== Comparative Results ===" << std::endl;
    std::cout << std::left << std::setw(15) << "Metric" 
              << std::setw(20) << "Unlimited (Spike)" 
              << std::setw(20) << "Limited (Smooth)" << std::endl;
    std::cout << std::string(55, '-') << std::endl;
    
    std::cout << std::left << std::setw(15) << "Write Time" 
              << std::setw(20) << (std::to_string(result_unlimited.write_duration_ms) + " ms")
              << std::to_string(result_limited.write_duration_ms) + " ms" << std::endl;

    std::ostringstream avg_unlim, avg_lim, p99_unlim, p99_lim;
    avg_unlim << std::fixed << std::setprecision(2) << result_unlimited.avg_read_latency_us << " us";
    avg_lim << std::fixed << std::setprecision(2) << result_limited.avg_read_latency_us << " us";
    p99_unlim << std::fixed << std::setprecision(2) << result_unlimited.p99_read_latency_us << " us";
    p99_lim << std::fixed << std::setprecision(2) << result_limited.p99_read_latency_us << " us";

    std::cout << std::left << std::setw(15) << "Read Avg Lat" 
              << std::setw(20) << avg_unlim.str() << avg_lim.str() << std::endl;

    std::cout << std::left << std::setw(15) << "Read P99 Lat" 
              << std::setw(20) << p99_unlim.str() << p99_lim.str() << std::endl;

    std::cout << "\nAnalysis:" << std::endl;

    // --- Validation Logic ---
    
    // 1. Check Rate Limit Accuracy
    // Expected time: 5MB / 2MB/s = 2.5 seconds = 2500 ms
    double expected_seconds = (double(write_mb) * 1024 * 1024) / limit_rate;
    long expected_ms = static_cast<long>(expected_seconds * 1000);
    // Allow 20% tolerance
    long tolerance_ms = static_cast<long>(expected_ms * 0.2); 

    bool accurate_rate = std::abs(result_limited.write_duration_ms - expected_ms) < tolerance_ms;
    
    if (accurate_rate) {
        std::cout << "PASS: Write duration matches rate limit target." << std::endl;
        std::cout << "  Expected: " << expected_ms << "ms (±" << tolerance_ms << "ms)" << std::endl;
        std::cout << "  Actual:   " << result_limited.write_duration_ms << "ms" << std::endl;
    } else {
        std::cout << "FAIL: Write duration outside expected range." << std::endl;
        std::cout << "  Expected: " << expected_ms << "ms (±" << tolerance_ms << "ms)" << std::endl;
        std::cout << "  Actual:   " << result_limited.write_duration_ms << "ms" << std::endl;
    }

    // 2. Check QoS Improvement (Latency)
    // Rate limiting should improve read latency compared to unlimited spike
    bool latency_improved = result_limited.avg_read_latency_us < result_unlimited.avg_read_latency_us;

    if (latency_improved) {
        std::cout << "PASS: Rate limiting improved read latency by " 
                  << (result_unlimited.avg_read_latency_us / result_limited.avg_read_latency_us) 
                  << "x times!" << std::endl;
    } else {
        std::cout << "WARN: Rate limiting did not improve latency (Background noise low?)." << std::endl;
    }

    // CLEANUP
    CleanDB("/tmp/lsm_test_mixed_on");
    CleanDB("/tmp/lsm_test_mixed_off");

    // Require both accuracy and improvement (or at least valid rate limiting) to pass
    if (accurate_rate && latency_improved) {
        return 0;
    } else {
        return 1;
    }
}

