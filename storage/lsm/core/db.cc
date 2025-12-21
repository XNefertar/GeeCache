#include "db.h"
#include <iostream>
#include <filesystem>
#include <chrono>
#include <fstream>

namespace lsm {

namespace fs = std::filesystem;

DB::DB(const std::string& path, const Options& options) 
    : path_(path), options_(options), stop_sync_(false) {
    if (!fs::exists(path)) {
        fs::create_directories(path);
    }
    
    std::string wal_path = path + "/wal.log";
    
    memtable_ = std::make_unique<MemTable>();
    
    Recover(wal_path);
    
    wal_ = std::make_unique<WAL>(wal_path);
    
    if (!options_.sync) {
        sync_thread_ = std::thread(&DB::BackgroundSync, this);
    }
    
    std::cout << "[C++] DB opened at " << path_ << std::endl;
}

DB::~DB() {
    stop_sync_ = true;
    if (sync_thread_.joinable()) {
        sync_thread_.join();
    }
    // Force final sync on close
    if (wal_) {
        wal_->Sync();
    }
    std::cout << "[C++] DB closed" << std::endl;
}

void DB::Put(const std::string& key, const std::string& value) {
    std::lock_guard<std::mutex> lock(mutex_);
    wal_->Append(key, value, false);
    if (options_.sync) {
        wal_->Sync();
    }
    memtable_->Put(key, value);
}

bool DB::Get(const std::string& key, std::string* value) {
    std::lock_guard<std::mutex> lock(mutex_);
    return memtable_->Get(key, value);
}

void DB::Delete(const std::string& key) {
    std::lock_guard<std::mutex> lock(mutex_);
    wal_->Append(key, "", true);
    if (options_.sync) {
        wal_->Sync();
    }
    memtable_->Delete(key);
}

void DB::BackgroundSync() {
    while (!stop_sync_) {
        std::this_thread::sleep_for(std::chrono::seconds(1));
        if (stop_sync_) break;
        if (wal_) {
            wal_->Sync();
        }
    }
}

void DB::Recover(const std::string& wal_path) {
    if (!fs::exists(wal_path)) return;

    std::ifstream file(wal_path, std::ios::binary);
    if (!file.is_open()) return;

    std::streampos valid_pos = 0;

    while (file.peek() != EOF) {
        char type;
        uint32_t klen;
        
        // Read header
        file.read(&type, 1);
        if (file.gcount() != 1) break;
        
        file.read(reinterpret_cast<char*>(&klen), sizeof(klen));
        if (file.gcount() != sizeof(klen)) break;
        
        // Read key
        std::string key(klen, '\0');
        file.read(&key[0], klen);
        if (file.gcount() != klen) break;
        
        if (type == 0) { // Put
            uint32_t vlen;
            file.read(reinterpret_cast<char*>(&vlen), sizeof(vlen));
            if (file.gcount() != sizeof(vlen)) break;
            
            std::string value(vlen, '\0');
            file.read(&value[0], vlen);
            if (file.gcount() != vlen) break;
            
            memtable_->Put(key, value);
        } else if (type == 1) { // Delete
            uint32_t zero;
            file.read(reinterpret_cast<char*>(&zero), sizeof(zero));
            if (file.gcount() != sizeof(zero)) break;
            
            memtable_->Delete(key);
        } else {
            // Unknown type, corrupted
            break;
        }
        
        // If we reached here, the record is valid
        valid_pos = file.tellg();
    }
    
    file.close();
    
    // Truncate corrupted data at the end
    try {
        uintmax_t file_size = fs::file_size(wal_path);
        if (static_cast<uintmax_t>(valid_pos) < file_size) {
            std::cerr << "[Recover] Truncating corrupted WAL from " << file_size << " to " << valid_pos << std::endl;
            fs::resize_file(wal_path, valid_pos);
        }
    } catch (const std::exception& e) {
        std::cerr << "[Recover] Failed to truncate WAL: " << e.what() << std::endl;
    }
}

} // namespace lsm
