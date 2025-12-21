#include "wal.h"
#include <iostream>
#include <fcntl.h>
#include <unistd.h>
#include <cstring>
#include <vector>

namespace lsm {

    WAL::WAL(const std::string& path) : path_(path), fd_(-1) {
        fd_ = ::open(path.c_str(), O_WRONLY | O_CREAT | O_APPEND, 0644);
        if (fd_ < 0) {
            std::cerr << "Failed to open WAL file: " << path << " Error: " << strerror(errno) << std::endl;
        }
    }

    WAL::~WAL() {
        if (fd_ >= 0) {
            ::close(fd_);
        }
    }

    void WAL::Append(const std::string& key, const std::string& value, bool is_delete) {
        std::lock_guard<std::mutex> lock(mutex_);
        if (fd_ < 0) return;

        // Simple format: type(1) | key_len(4) | key | val_len(4) | val
        // type: 0 = put, 1 = delete
        
        char type = is_delete ? 1 : 0;
        uint32_t klen = key.size();
        uint32_t vlen = value.size();

        // Use a buffer to minimize syscalls
        std::vector<char> buffer;
        // Estimate size: 1 + 4 + klen + 4 + vlen
        buffer.reserve(1 + 4 + klen + 4 + vlen);

        buffer.push_back(type);
        
        const char* klen_ptr = reinterpret_cast<const char*>(&klen);
        buffer.insert(buffer.end(), klen_ptr, klen_ptr + 4);
        
        buffer.insert(buffer.end(), key.begin(), key.end());
        
        if (!is_delete) {
            const char* vlen_ptr = reinterpret_cast<const char*>(&vlen);
            buffer.insert(buffer.end(), vlen_ptr, vlen_ptr + 4);
            buffer.insert(buffer.end(), value.begin(), value.end());
        } else {
            uint32_t zero = 0;
            const char* zero_ptr = reinterpret_cast<const char*>(&zero);
            buffer.insert(buffer.end(), zero_ptr, zero_ptr + 4);
        }
        
        ssize_t written = ::write(fd_, buffer.data(), buffer.size());
        if (written < 0 || static_cast<size_t>(written) != buffer.size()) {
             std::cerr << "Failed to write to WAL: " << strerror(errno) << std::endl;
        }
        // No fsync here by default, relying on OS cache (Scheme A)
    }

    void WAL::Sync() {
        std::lock_guard<std::mutex> lock(mutex_);
        if (fd_ >= 0) {
            ::fsync(fd_);
        }
    }

} // namespace lsm
