#include "core/db.h"
#include <cassert>
#include <iostream>
#include <filesystem>
#include <vector>
#include <thread>
#include <atomic>
#include <chrono>

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

int main() {
    TestBasic();
    TestRecovery();
    TestConcurrencyCorrectness();
    std::cout << "All Correctness Tests Passed!" << std::endl;
    return 0;
}
