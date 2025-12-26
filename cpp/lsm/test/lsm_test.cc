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
    std::cout << "Running TestBasic..." << std::endl;
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
    std::cout << "TestBasic Passed!" << std::endl;
}

void TestRecovery() {
    std::cout << "Running TestRecovery..." << std::endl;
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
    std::cout << "TestRecovery Passed!" << std::endl;
}

void TestConcurrencyCorrectness() {
    std::cout << "Running TestConcurrencyCorrectness..." << std::endl;
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
    std::cout << "TestConcurrencyCorrectness Passed!" << std::endl;
}

void TestFlush() {
    std::cout << "Running TestFlush..." << std::endl;
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
    std::cout << "TestFlush Passed!" << std::endl;
}

void TestFlushRecovery() {
    std::cout << "Running TestFlushRecovery..." << std::endl;
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
    std::cout << "TestFlushRecovery Passed!" << std::endl;
}

void TestReadWriteConcurrency() {
    std::cout << "Running TestReadWriteConcurrency..." << std::endl;
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
    std::cout << "TestReadWriteConcurrency Passed!" << std::endl;
}

int main() {
    TestBasic();
    TestRecovery();
    TestConcurrencyCorrectness();
    TestFlush();
    TestFlushRecovery();
    TestReadWriteConcurrency();
    std::cout << "All Correctness Tests Passed!" << std::endl;
    return 0;
}
