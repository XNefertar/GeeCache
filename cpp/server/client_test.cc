#include "../rpc/client.h"
#include <iostream>
#include <cassert>
#include <cstring>
#include <vector>

void EncodePutBody(const std::string& key, const std::string& val, std::string& out) {
    uint32_t k_len = key.size();
    out.resize(4 + key.size() + val.size());
    char* ptr = out.data();
    memcpy(ptr, &k_len, 4);
    memcpy(ptr + 4, key.data(), key.size());
    memcpy(ptr + 4 + key.size(), val.data(), val.size());
}

int main() {
    try {
        geecache::rpc::Client client("127.0.0.1", 9000);
        
        std::string key = "key1";
        std::string val = "value1";

        // Put
        std::string args_put;
        EncodePutBody(key, val, args_put);
        std::string reply_put;
        client.Call("put", args_put, reply_put);
        std::cout << "Put done" << std::endl;

        // Get
        std::string reply_get;
        client.Call("get", key, reply_get); // Body is just key
        std::cout << "Get result: " << reply_get << std::endl;

        if (reply_get == val) {
            std::cout << "Test Passed!" << std::endl;
        } else {
            std::cout << "Test Failed! Expected " << val << ", got " << reply_get << std::endl;
            return 1;
        }

        // Delete
        std::string reply_del;
        client.Call("delete", key, reply_del); // Body is just key
        std::cout << "Delete done" << std::endl;

        // Get again (should fail or be empty)
        try {
            std::string reply_get2;
            client.Call("get", key, reply_get2);
            std::cout << "Get result after delete: " << reply_get2 << std::endl;
        } catch (const std::exception& e) {
             std::cout << "Test Passed (Get failed as expected): " << e.what() << std::endl;
        }

    } catch (const std::exception& e) {
        std::cerr << "Error: " << e.what() << std::endl;
        return 1;
    }
    return 0;
}
