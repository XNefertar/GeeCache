#include "core/db.h"
#include <iostream>
#include <vector>
#include <thread>
#include <chrono>
#include <atomic>
#include <random>
#include <filesystem>
#include <iomanip>

namespace fs = std::filesystem;
using namespace lsm;

void CleanDB(const std::string& path) {
    if (fs::exists(path)) {
        fs::remove_all(path);
    }
}

struct Stats {
    std::atomic<uint64_t> ops_completed{0};
    std::atomic<uint64_t> total_latency_ns{0};
};

void Benchmark(const std::string& name, int num_threads, int num_ops, int value_size, bool is_write) {
    std::string db_path = "/tmp/lsm_bench_" + name;
    CleanDB(db_path);
    
    // Disable sync for pure throughput test, enable for durability test
    Options options;
    options.sync = false; 
    DB db(db_path, options);

    // Pre-fill for read test
    if (!is_write) {
        std::cout << "Pre-filling DB for read benchmark..." << std::endl;
        std::vector<std::thread> fill_threads;
        int fill_threads_count = 4;
        int ops_per_fill = num_ops / fill_threads_count;
        for (int i = 0; i < fill_threads_count; ++i) {
            fill_threads.emplace_back([&db, i, ops_per_fill, value_size]() {
                std::string val(value_size, 'x');
                for (int j = 0; j < ops_per_fill; ++j) {
                    std::string key = "key_" + std::to_string(i * ops_per_fill + j); // Unique keys
                    db.Put(key, val);
                }
            });
        }
        for (auto& t : fill_threads) t.join();
    }

    std::cout << "Starting Benchmark: " << name << " (Threads: " << num_threads 
              << ", Ops: " << num_ops << ", ValSize: " << value_size << "B)" << std::endl;

    Stats stats;
    std::vector<std::thread> threads;
    auto start_time = std::chrono::high_resolution_clock::now();

    int ops_per_thread = num_ops / num_threads;

    for (int i = 0; i < num_threads; ++i) {
        threads.emplace_back([&db, &stats, i, ops_per_thread, value_size, is_write, num_ops]() {
            std::string val(value_size, 'v');
            // Random read generator
            std::mt19937 rng(i);
            std::uniform_int_distribution<int> dist(0, num_ops - 1);

            for (int j = 0; j < ops_per_thread; ++j) {
                auto op_start = std::chrono::high_resolution_clock::now();
                
                if (is_write) {
                    // Write unique keys to avoid overwriting too much (or random keys)
                    // Let's use sequential keys per thread to ensure uniqueness for verification if needed
                    std::string key = "key_" + std::to_string(i) + "_" + std::to_string(j);
                    db.Put(key, val);
                } else {
                    // Random read
                    std::string key = "key_" + std::to_string(dist(rng));
                    std::string res;
                    db.Get(key, &res);
                }

                auto op_end = std::chrono::high_resolution_clock::now();
                stats.total_latency_ns += std::chrono::duration_cast<std::chrono::nanoseconds>(op_end - op_start).count();
                stats.ops_completed++;
            }
        });
    }

    for (auto& t : threads) t.join();

    auto end_time = std::chrono::high_resolution_clock::now();
    auto duration_sec = std::chrono::duration_cast<std::chrono::duration<double>>(end_time - start_time).count();

    uint64_t total_ops = stats.ops_completed;
    double throughput = total_ops / duration_sec;
    double avg_latency_us = (double)stats.total_latency_ns / total_ops / 1000.0;

    std::cout << "------------------------------------------------" << std::endl;
    std::cout << "Results for " << name << ":" << std::endl;
    std::cout << "  Duration:     " << std::fixed << std::setprecision(2) << duration_sec << " s" << std::endl;
    std::cout << "  Throughput:   " << std::fixed << std::setprecision(2) << throughput << " ops/sec" << std::endl;
    std::cout << "  Avg Latency:  " << std::fixed << std::setprecision(2) << avg_latency_us << " us" << std::endl;
    std::cout << "------------------------------------------------" << std::endl;

    CleanDB(db_path);
}

int main() {
    // 1. Write Benchmark (High Concurrency)
    Benchmark("Write_HighConcurrency", 8, 100000, 100, true);

    // 2. Read Benchmark (High Concurrency)
    Benchmark("Read_HighConcurrency", 8, 100000, 100, false);

    // 3. Mixed / Larger Value
    Benchmark("Write_LargeValue", 4, 50000, 4096, true);

    return 0;
}
