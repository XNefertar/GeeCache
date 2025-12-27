#include "../rpc/server.h"
#include "../lsm/include/lsm.h"
#include <iostream>
#include <string>
#include <vector>
#include <map>
#include <mutex>
#include <filesystem>
#include <cstring>

namespace fs = std::filesystem;

class CacheServer {
public:
    CacheServer(const std::string& base_dir) : base_dir_(base_dir) {
        fs::create_directories(base_dir_);
    }

    ~CacheServer() {
        for (auto& pair : dbs_) {
            lsm_db_close(pair.second);
        }
    }

    void Get(const std::string& args, std::string& reply) {
        std::string group, key;
        if (!ParseArgs(args, group, key)) {
            throw std::runtime_error("Invalid arguments");
        }

        lsm_db_t* db = GetDB(group);
        if (!db) {
            throw std::runtime_error("Failed to open group DB");
        }

        size_t vallen = 0;
        char* err = nullptr;
        char* val = lsm_get(db, key.data(), key.size(), &vallen, &err);

        if (err) {
            std::string msg(err);
            lsm_free(err);
            throw std::runtime_error(msg);
        }

        if (val) {
            reply.assign(val, vallen);
            lsm_free(val);
        }
    }

    void Set(const std::string& args, std::string& reply) {
        std::string group, key, val;
        if (!ParseArgsSet(args, group, key, val)) {
            throw std::runtime_error("Invalid arguments");
        }

        lsm_db_t* db = GetDB(group);
        if (!db) {
             throw std::runtime_error("Failed to open group DB");
        }

        char* err = nullptr;
        lsm_put(db, key.data(), key.size(), val.data(), val.size(), &err);
        
        if (err) {
            std::string msg(err);
            lsm_free(err);
            throw std::runtime_error(msg);
        }
    }

    void Remove(const std::string& args, std::string& reply) {
        std::string group, key;
        if (!ParseArgs(args, group, key)) {
            throw std::runtime_error("Invalid arguments");
        }

        lsm_db_t* db = GetDB(group);
        if (!db) {
             return;
        }

        char* err = nullptr;
        lsm_delete(db, key.data(), key.size(), &err);
        if (err) {
            std::string msg(err);
            lsm_free(err);
            throw std::runtime_error(msg);
        }
    }

private:
    lsm_db_t* GetDB(const std::string& group) {
        std::lock_guard<std::mutex> lock(mu_);
        auto it = dbs_.find(group);
        if (it != dbs_.end()) return it->second;

        std::string path = base_dir_ + "/" + group;
        lsm_options_t* opts = lsm_options_create();
        lsm_options_set_create_if_missing(opts, 1);
        
        char* err = nullptr;
        lsm_db_t* db = lsm_db_open(opts, path.c_str(), &err);
        lsm_options_destroy(opts);

        if (err) {
            std::cerr << "Failed to open DB " << group << ": " << err << std::endl;
            lsm_free(err);
            return nullptr;
        }

        dbs_[group] = db;
        return db;
    }

    bool ParseArgs(const std::string& data, std::string& group, std::string& key) {
        if (data.size() < 8) return false;
        const char* ptr = data.data();
        size_t remaining = data.size();

        uint32_t g_len = *reinterpret_cast<const uint32_t*>(ptr);
        ptr += 4; remaining -= 4;
        if (remaining < g_len) return false;
        group.assign(ptr, g_len);
        ptr += g_len; remaining -= g_len;

        if (remaining < 4) return false;
        uint32_t k_len = *reinterpret_cast<const uint32_t*>(ptr);
        ptr += 4; remaining -= 4;
        if (remaining < k_len) return false;
        key.assign(ptr, k_len);
        
        return true;
    }

    bool ParseArgsSet(const std::string& data, std::string& group, std::string& key, std::string& val) {
        if (data.size() < 12) return false;
        const char* ptr = data.data();
        size_t remaining = data.size();

        uint32_t g_len = *reinterpret_cast<const uint32_t*>(ptr);
        ptr += 4; remaining -= 4;
        if (remaining < g_len) return false;
        group.assign(ptr, g_len);
        ptr += g_len; remaining -= g_len;

        if (remaining < 4) return false;
        uint32_t k_len = *reinterpret_cast<const uint32_t*>(ptr);
        ptr += 4; remaining -= 4;
        if (remaining < k_len) return false;
        key.assign(ptr, k_len);
        ptr += k_len; remaining -= k_len;

        if (remaining < 4) return false;
        uint32_t v_len = *reinterpret_cast<const uint32_t*>(ptr);
        ptr += 4; remaining -= 4;
        if (remaining < v_len) return false;
        val.assign(ptr, v_len);

        return true;
    }

    std::string base_dir_;
    std::map<std::string, lsm_db_t*> dbs_;
    std::mutex mu_;
};

int main(int argc, char** argv) {
    int port = 9000;
    if (argc > 1) port = std::atoi(argv[1]);

    CacheServer cache("data");
    geecache::rpc::Server server;

    server.Register("GroupCache.Get", [&](const std::string& args, std::string& reply) {
        cache.Get(args, reply);
    });

    server.Register("GroupCache.Set", [&](const std::string& args, std::string& reply) {
        cache.Set(args, reply);
    });

    server.Register("GroupCache.Remove", [&](const std::string& args, std::string& reply) {
        cache.Remove(args, reply);
    });

    server.Run(port);
    return 0;
}
