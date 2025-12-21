#include "db.h"
#include <iostream>
#include <filesystem>

namespace lsm {

namespace fs = std::filesystem;

DB::DB(const std::string& path) : path_(path) {
    if (!fs::exists(path)) {
        fs::create_directories(path);
    }
    
    std::string wal_path = path + "/wal.log";
    
    memtable_ = std::make_unique<MemTable>();
    
    Recover(wal_path);
    
    wal_ = std::make_unique<WAL>(wal_path);
    
    std::cout << "[C++] DB opened at " << path_ << std::endl;
}

DB::~DB() {
    std::cout << "[C++] DB closed" << std::endl;
}

void DB::Put(const std::string& key, const std::string& value) {
    std::lock_guard<std::mutex> lock(mutex_);
    wal_->Append(key, value, false);
    memtable_->Put(key, value);
}

bool DB::Get(const std::string& key, std::string* value) {
    std::lock_guard<std::mutex> lock(mutex_);
    return memtable_->Get(key, value);
}

void DB::Delete(const std::string& key) {
    std::lock_guard<std::mutex> lock(mutex_);
    wal_->Append(key, "", true);
    memtable_->Delete(key);
}

void DB::Recover(const std::string& wal_path) {
    if (!fs::exists(wal_path)) return;

    std::ifstream file(wal_path, std::ios::binary);
    if (!file.is_open()) return;

    while (file.peek() != EOF) {
        char type;
        uint32_t klen;
        
        file.read(&type, 1);
        if (file.gcount() != 1) break;
        
        file.read(reinterpret_cast<char*>(&klen), sizeof(klen));
        if (file.gcount() != sizeof(klen)) break;
        
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
            memtable_->Delete(key);
        }
    }
}

} // namespace lsm
