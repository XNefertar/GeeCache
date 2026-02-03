#include "../logging/async_logging.h"
#include "../logging/logging.h"
#include <vector>
#include <thread>
#include <chrono>
#include <atomic>
#include <iostream>
#include <string>

// Simple benchmark for logging
void bench(bool async, int threads, int num_logs_per_thread) {
    std::atomic<bool> start_flag{false};
    std::vector<std::thread> workers;
    
    // Determine log file name
    std::string filename = async ? "bench_async.log" : "bench_sync.log";
    
    // Setup logging
    if (async) {
        lsm::setupAsyncLogging(filename);
    } else {
        lsm::Logger::setOutput(lsm::ConsoleOutput);
    }

    for (int i = 0; i < threads; ++i) {
        workers.emplace_back([&, i]() {
            while (!start_flag) {
                std::this_thread::yield();
            }
            
            std::string payload(100, 'X'); // 100 bytes payload
            for (int j = 0; j < num_logs_per_thread; ++j) {
                LOG_INFO << "Thread " << i << " msg " << j << " payload: " << payload << " int:" << j*i << " double:" << 3.14159;
            }
        });
    }

    start_flag = true;
    auto t1 = std::chrono::high_resolution_clock::now();

    for (auto& t : workers) {
        t.join();
    }

    auto t2 = std::chrono::high_resolution_clock::now();
    
    // For async, the main thread logs are "submission" time, not disk write time.
    // The background worker might still be writing.
    // However, LogStream destruction calls append(), checking submission latency.

    double elapsed_sec = std::chrono::duration<double>(t2 - t1).count();
    long total_logs = static_cast<long>(threads) * num_logs_per_thread;
    
    std::cout << (async ? "Async" : "Sync/Console") << " Logging: " 
              << total_logs << " logs in " << elapsed_sec << "s. "
              << "Average latency (submission): " << (elapsed_sec * 1e6 / total_logs) << "us/log. "
              << "Throughput: " << (total_logs / elapsed_sec) << " logs/sec." << std::endl;
}

int main(int argc, char* argv[]) {
    int threads = 4;
    int count = 100000;
    
    if (argc > 1) threads = std::atoi(argv[1]);
    if (argc > 2) count = std::atoi(argv[2]);

    std::cout << "Benchmarking with " << threads << " threads, " << count << " logs each." << std::endl;

    // We only test Async here as Sync(Stdout) depends heavily on terminal speed.
    bench(true, threads, count);
    
    // Allow background worker to finish flushing
    std::this_thread::sleep_for(std::chrono::seconds(2));
    
    return 0;
}
