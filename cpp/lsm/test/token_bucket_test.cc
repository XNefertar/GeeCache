#include "core/token_bucket.h"
#include <iostream>
#include <cassert>
#include <thread>
#include <chrono>
#include <vector>

using namespace lsm;

void TestBasicConsumption() {
    std::cout << "[Test] Basic Consumption... ";
    // Capacity 100, Refill 10 bytes/sec
    TokenBucket bucket(100.0, 10.0);
    
    // Initially full (100 tokens)
    assert(bucket.Consume(50, 0) == true); // 50 left
    assert(bucket.Consume(40, 0) == true); // 10 left
    assert(bucket.Consume(20, 0) == false); // Need 20, have 10 -> Fail
    
    std::cout << "PASSED" << std::endl;
}

void TestRefill() {
    std::cout << "[Test] Refill Rate... ";
    // 20 bytes/sec => 2 bytes per 100ms
    TokenBucket bucket(10.0, 20.0);
    
    // Consume all
    bucket.Consume(10, 0);
    assert(bucket.Consume(1, 0) == false);
    
    // Wait 150ms (should refill ~3 bytes)
    std::this_thread::sleep_for(std::chrono::milliseconds(150));
    
    assert(bucket.Consume(2, 0) == true);
    
    std::cout << "PASSED" << std::endl;
}

void TestBlocking() {
    std::cout << "[Test] Blocking Consume... ";
    TokenBucket bucket(10.0, 10.0);
    
    bucket.Consume(10, 0); // Empty
    
    // Request 1 byte, wait up to 200ms. 
    // Refill rate is 10 bytes/sec -> 1 byte every 100ms.
    auto start = std::chrono::steady_clock::now();
    bool success = bucket.Consume(1, 200); 
    auto end = std::chrono::steady_clock::now();
    
    assert(success == true);
    auto duration_ms = std::chrono::duration_cast<std::chrono::milliseconds>(end - start).count();
    
    // Should have waited at least ~80ms (allowing for scheduler jitter)
    assert(duration_ms >= 80);
    
    std::cout << "PASSED (Waited " << duration_ms << "ms)" << std::endl;
}

void TestDynamicRate() {
    std::cout << "[Test] Dynamic Rate Adjustment... ";
    TokenBucket bucket(100.0, 1.0); // Very slow: 1 byte/sec
    bucket.Consume(100, 0); // Empty
    
    // Speed up to 200 bytes/sec
    bucket.SetRefillRate(200.0);
    
    // Sleep 50ms -> should get ~10 bytes
    std::this_thread::sleep_for(std::chrono::milliseconds(50));
    assert(bucket.Consume(5, 0) == true);
    
    std::cout << "PASSED" << std::endl;
}

void TestRequest() {
    std::cout << "[Test] Request Blocking... ";
    // 100 bytes/sec
    TokenBucket bucket(10.0, 100.0);
    bucket.Consume(10, 0); // Empty

    // Request 50 bytes. Rate 100/s -> need 0.5 sec wait.
    auto start = std::chrono::steady_clock::now();
    bucket.Request(50);
    auto end = std::chrono::steady_clock::now();

    auto duration_ms = std::chrono::duration_cast<std::chrono::milliseconds>(end - start).count();
    
    // Check if wait is around 500ms
    assert(duration_ms >= 450);
    
    std::cout << "PASSED (Waited " << duration_ms << "ms for 50 bytes @ 100B/s)" << std::endl;
}

int main() {
    std::cout << "=== Running TokenBucket Tests ===" << std::endl;
    TestBasicConsumption();
    TestRefill();
    TestBlocking();
    TestDynamicRate();
    TestRequest();
    std::cout << "=== All TokenBucket Tests Passed ===" << std::endl;
    return 0;
}
