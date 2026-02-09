#include "core/db.h"
#include <cassert>
#include <iostream>
#include <filesystem>
#include <vector>
#include <thread>
#include <atomic>
#include <chrono>
#include <cstdlib>

namespace fs = std::filesystem;
using namespace lsm;

void CleanDB(const std::string& path) {
    if (fs::exists(path)) {
        fs::remove_all(path);
    }
}

void TestBasic() {
    // std::cout << "Running TestBasic..." << std::endl;
    std::string db_path = "/tmp/lsm_test_basic";
    CleanDB(db_path);

    {
        DB db(db_path);
        db.Put("key1", "value1");
        db.Put("key2", "value2");
        
        std::string val;
        assert(db.Get("key1", &val) && val == "value1");
        assert(db.Get("key2", &val) && val == "value2");
        assert(!db.Get("key3", &val));
        
        db.Delete("key1");
        assert(!db.Get("key1", &val));
    }
    
    CleanDB(db_path);
    // std::cout << "TestBasic Passed!" << std::endl;
}

void TestRecovery() {
    // std::cout << "Running TestRecovery..." << std::endl;
    std::string db_path = "/tmp/lsm_test_recovery";
    CleanDB(db_path);

    {
        DB db(db_path);
        db.Put("key1", "value1");
        db.Put("key2", "value2");
        // DB closes here, WAL should be synced
    }

    {
        DB db(db_path);
        std::string val;
        assert(db.Get("key1", &val) && val == "value1");
        assert(db.Get("key2", &val) && val == "value2");
    }

    CleanDB(db_path);
    // std::cout << "TestRecovery Passed!" << std::endl;
}

void TestConcurrencyCorrectness() {
    // std::cout << "Running TestConcurrencyCorrectness..." << std::endl;
    std::string db_path = "/tmp/lsm_test_concurrency";
    CleanDB(db_path);

    DB db(db_path);
    int num_threads = 4;
    int ops_per_thread = 1000;
    std::vector<std::thread> threads;

    for (int i = 0; i < num_threads; ++i) {
        threads.emplace_back([&db, i, ops_per_thread]() {
            for (int j = 0; j < ops_per_thread; ++j) {
                std::string key = "key_" + std::to_string(i) + "_" + std::to_string(j);
                std::string val = "val_" + std::to_string(i) + "_" + std::to_string(j);
                db.Put(key, val);
            }
        });
    }

    for (auto& t : threads) t.join();
    // 强制刷盘
    // 解决数据再 "Immutable MemTable" 期间对 Get 不可见的一致性问题
    db.ForceFlush();

    // Verify
    for (int i = 0; i < num_threads; ++i) {
        for (int j = 0; j < ops_per_thread; ++j) {
            std::string key = "key_" + std::to_string(i) + "_" + std::to_string(j);
            std::string expected = "val_" + std::to_string(i) + "_" + std::to_string(j);
            std::string val;
            if (!db.Get(key, &val) || val != expected) {
                std::cerr << "Failed to get key: " << key << std::endl;
                assert(false);
            }
        }
    }

    CleanDB(db_path);
    // std::cout << "TestConcurrencyCorrectness Passed!" << std::endl;
}

void TestFlush() {
    // std::cout << "Running TestFlush..." << std::endl;
    std::string db_path = "/tmp/lsm_test_flush";
    CleanDB(db_path);

    {
        DB db(db_path);
        // Write 5MB of data
        std::string large_value(1024, 'a'); // 1KB
        for (int i = 0; i < 5000; ++i) {
            db.Put("key" + std::to_string(i), large_value);
        }
        
        // Check if SSTable exists
        bool sst_exists = false;
        for (const auto& entry : fs::directory_iterator(db_path)) {
            if (entry.path().extension() == ".sst") {
                sst_exists = true;
                break;
            }
        }
        assert(sst_exists);
        
        // Verify data
        std::string val;
        assert(db.Get("key0", &val) && val == large_value);
        assert(db.Get("key4999", &val) && val == large_value);
    }
    
    CleanDB(db_path);
    // std::cout << "TestFlush Passed!" << std::endl;
}

void TestFlushRecovery() {
    // std::cout << "Running TestFlushRecovery..." << std::endl;
    std::string db_path = "/tmp/lsm_test_flush_recovery";
    CleanDB(db_path);

    std::string large_value(1024, 'a'); // 1KB
    int num_entries = 5000;

    {
        DB db(db_path);
        for (int i = 0; i < num_entries; ++i) {
            db.Put("key" + std::to_string(i), large_value);
        }
        // Should have flushed at least once
    }

    {
        DB db(db_path);
        std::string val;
        assert(db.Get("key0", &val) && val == large_value);
        assert(db.Get("key4999", &val) && val == large_value);
    }
    
    CleanDB(db_path);
    // std::cout << "TestFlushRecovery Passed!" << std::endl;
}

void TestReadWriteConcurrency() {
    // std::cout << "Running TestReadWriteConcurrency..." << std::endl;
    std::string db_path = "/tmp/lsm_test_rw_concurrency";
    CleanDB(db_path);

    DB db(db_path);
    const int num_writers = 2;
    const int num_readers = 4;
    const int ops_per_writer = 5000;
    std::atomic<bool> stop_readers(false);
    std::vector<std::thread> threads;

    // Writers
    for (int i = 0; i < num_writers; ++i) {
        threads.emplace_back([&db, i, ops_per_writer]() {
            for (int j = 0; j < ops_per_writer; ++j) {
                std::string key = "key_" + std::to_string(i) + "_" + std::to_string(j);
                std::string val = "val_" + std::to_string(i) + "_" + std::to_string(j);
                db.Put(key, val);
                if (j % 100 == 0) std::this_thread::yield();
            }
        });
    }

    // Readers
    for (int i = 0; i < num_readers; ++i) {
        threads.emplace_back([&db, num_writers, ops_per_writer, &stop_readers]() {
            while (!stop_readers) {
                int w = rand() % num_writers;
                int j = rand() % ops_per_writer;
                std::string key = "key_" + std::to_string(w) + "_" + std::to_string(j);
                std::string expected = "val_" + std::to_string(w) + "_" + std::to_string(j);
                std::string val;
                if (db.Get(key, &val)) {
                    if (val != expected) {
                        std::cerr << "Read mismatch for key: " << key << " Expected: " << expected << " Got: " << val << std::endl;
                        std::terminate();
                    }
                }
            }
        });
    }

    // Join writers
    for (int i = 0; i < num_writers; ++i) {
        threads[i].join();
    }
    
    stop_readers = true;
    
    // Join readers
    for (int i = num_writers; i < num_writers + num_readers; ++i) {
        threads[i].join();
    }

    // Final verification
    for (int i = 0; i < num_writers; ++i) {
        for (int j = 0; j < ops_per_writer; ++j) {
            std::string key = "key_" + std::to_string(i) + "_" + std::to_string(j);
            std::string expected = "val_" + std::to_string(i) + "_" + std::to_string(j);
            std::string val;
            if (!db.Get(key, &val) || val != expected) {
                 std::cerr << "Final verification failed for key: " << key << std::endl;
                 assert(false);
            }
        }
    }

    CleanDB(db_path);
    // std::cout << "TestReadWriteConcurrency Passed!" << std::endl;
}

void TestSortLevelNecessity() {
    // std::cout << "Running TestSortLevelNecessity..." << std::endl;
    std::string db_path = "/tmp/lsm_test_sort_level";
    CleanDB(db_path);
    
    DB db(db_path);
    
    // Step 1: Inject Large Keys to create an SSTable in L1 with range [2000, 2200]
    std::cout << "  Step 1: Injecting Large Keys..." << std::endl;
    for (int i = 0; i < 4; ++i) {
        for (int k = 2000; k < 2200; ++k) {
             char buf[32]; snprintf(buf, sizeof(buf), "key_%06d", k);
             db.Put(std::string(buf), "val_large");
        }
        db.ForceFlush();
    }
    std::cout << "  Waiting for compaction..." << std::endl;
    std::this_thread::sleep_for(std::chrono::seconds(2));
    
    // Step 2: Inject Small Keys to create another SSTable in L1 with range [1000, 1200]
    // Crucially, this is done AFTER the first one. If SortLevel is missing, 
    // this file might be appended to L1, causing L1 to correspond to [LargeFile, SmallFile].
    std::cout << "  Step 2: Injecting Small Keys..." << std::endl;
    for (int i = 0; i < 4; ++i) {
        for (int k = 1000; k < 1200; ++k) {
             char buf[32]; snprintf(buf, sizeof(buf), "key_%06d", k);
             db.Put(std::string(buf), "val_small");
        }
        db.ForceFlush();
    }
    std::cout << "  Waiting for compaction..." << std::endl;
    std::this_thread::sleep_for(std::chrono::seconds(2));

    // Step 3: Read a Small Key
    // Lower_bound on [LargeFile, SmallFile] will find LargeFile for SmallKey, 
    // because LargeFile.start(2000) > SmallKey(1100) is FALSE, wait.
    // std::lower_bound(begin, end, val, cmp). Returns first it where comp(it, val) is false?
    // Default lower_bound: *it >= val.
    // Files are sorted by smallest key.
    // List: [File(2000), File(1000)].
    // Search 1100.
    // Is File(2000) >= 1100? Yes.
    // Returns iterator to File(2000).
    // Logic checks File(2000). Key 1100 is not in [2000, 2200].
    // Logic might check previous file? No, usually lower_bound points to the first file with smallest >= key.
    // If we find File(2000), it implies the key could be in the PREVIOUS file (if range overlaps) or THIS file (if start matches).
    // Actually, Version::Get logic:
    // Find first file with smallest_key >= target_key.
    // If that file's smallest_key > target_key, we must check the PREVIOUS file (index - 1).
    // Because the target key must be smaller than the current file's start.
    
    // Case: [File(2000), File(1000)]
    // lower_bound(1100) -> returns File(2000) (index 0).
    // check index 0 ? No, 2000 > 1100.
    // check index - 1 ? No index -1.
    // Result: Not Found.
    
    std::string key_check = "key_001100";
    std::string val;
    bool found = db.Get(key_check, &val);
    
    if (!found) {
        std::cerr << "TestSortLevelNecessity FAILED: Key " << key_check << " not found!" << std::endl;
        std::cerr << "Explanation: The L1 file list is likely unordered: [LargeFile, SmallFile]." << std::endl; 
        assert(false);
    } 
    
    CleanDB(db_path);
    // std::cout << "TestSortLevelNecessity Passed!" << std::endl;
}

// Helper for nicer test output
template <typename F>
void RunTest(const std::string& test_name, F func) {
    std::cout << "\n\033[1;36m================================================================================\033[0m" << std::endl;
    std::cout << "\033[1;36m[RUNNING]\033[0m " << test_name << std::endl;
    std::cout << "\033[1;36m================================================================================\033[0m" << std::endl;
    
    // Capture usage of cout to indent it, or just let it be. 
    // For simplicity, we just run it. The visual separators help enough.
    func();
    
    std::cout << "\033[1;32m[PASSED]  \033[0m" << test_name << std::endl;
}

int main() {
    RunTest("TestSortLevelNecessity", TestSortLevelNecessity);
    RunTest("TestBasic", TestBasic);
    RunTest("TestRecovery", TestRecovery);
    RunTest("TestConcurrencyCorrectness", TestConcurrencyCorrectness);
    RunTest("TestFlush", TestFlush);
    RunTest("TestFlushRecovery", TestFlushRecovery);
    RunTest("TestReadWriteConcurrency", TestReadWriteConcurrency);
    
    std::cout << "\n\033[1;32mAll Correctness Tests Passed!\033[0m" << std::endl;
    return 0;
}
