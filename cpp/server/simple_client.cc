#include "../rpc/client.h"
#include <iostream>
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

int main(int argc, char** argv) {
    if (argc < 4) {
        std::cerr << "Usage: " << argv[0] << " <host> <port> <command> [args...]" << std::endl;
        std::cerr << "Commands:" << std::endl;
        std::cerr << "  put <key> <value>" << std::endl;
        std::cerr << "  get <key>" << std::endl;
        std::cerr << "  delete <key>" << std::endl;
        return 1;
    }

    std::string host = argv[1];
    int port = std::stoi(argv[2]);
    std::string cmd = argv[3];

    try {
        geecache::rpc::Client client(host, port);
        std::string reply;

        if (cmd == "put") {
            if (argc != 6) {
                std::cerr << "Usage: put <key> <value>" << std::endl;
                return 1;
            }
            std::string key = argv[4];
            std::string val = argv[5];
            std::string args;
            EncodePutBody(key, val, args);
            client.Call("put", args, reply);
            std::cout << "OK" << std::endl;
        } else if (cmd == "get") {
            if (argc != 5) {
                std::cerr << "Usage: get <key>" << std::endl;
                return 1;
            }
            std::string key = argv[4];
            client.Call("get", key, reply);
            std::cout << reply << std::endl;
        } else if (cmd == "delete") {
            if (argc != 5) {
                std::cerr << "Usage: delete <key>" << std::endl;
                return 1;
            }
            std::string key = argv[4];
            client.Call("delete", key, reply);
            std::cout << "OK" << std::endl;
        } else {
            std::cerr << "Unknown command: " << cmd << std::endl;
            return 1;
        }

    } catch (const std::exception& e) {
        std::cerr << "Error: " << e.what() << std::endl;
        return 1;
    }
    return 0;
}
