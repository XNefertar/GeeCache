#include "wal.h"
#include <iostream>

namespace lsm {

    WAL::WAL(const std::string& path) : path_(path) {
        file_.open(path, std::ios::app | std::ios::binary);
        if (!file_.is_open()) {
            std::cerr << "Failed to open WAL file: " << path << std::endl;
        }
    }

    WAL::~WAL() {
        if (file_.is_open()) {
            file_.close();
        }
    }

    void WAL::Append(const std::string& key, const std::string& value, bool is_delete) {
        std::lock_guard<std::mutex> lock(mutex_);
        if (!file_.is_open()) return;

        // Simple format: type(1) | key_len(4) | key | val_len(4) | val
        // type: 0 = put, 1 = delete
        
        char type = is_delete ? 1 : 0;
        uint32_t klen = key.size();
        uint32_t vlen = value.size();

        file_.write(&type, 1);
        file_.write(reinterpret_cast<char*>(&klen), sizeof(klen));
        file_.write(key.data(), klen);
        
        if (!is_delete) {
            file_.write(reinterpret_cast<char*>(&vlen), sizeof(vlen));
            file_.write(value.data(), vlen);
        } else {
            // For delete, we can skip value or write 0 length
            uint32_t zero = 0;
            file_.write(reinterpret_cast<char*>(&zero), sizeof(zero));
        }
        
        file_.flush();
    }

} // namespace lsm
