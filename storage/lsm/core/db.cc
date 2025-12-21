#include "db.h"
#include <iostream>

namespace lsm {

DB::DB(const std::string& path) : path_(path) {
    std::cout << "[C++] DB opened at " << path_ << std::endl;
}

DB::~DB() {
    std::cout << "[C++] DB closed" << std::endl;
}

void DB::Put(const std::string& key, const std::string& value) {
    std::lock_guard<std::mutex> lock(mutex_);
    std::cout << "[C++] Put key: " << key << ", value: " << value << std::endl;
    data_[key] = value;
}

bool DB::Get(const std::string& key, std::string* value) {
    std::lock_guard<std::mutex> lock(mutex_);
    std::cout << "[C++] Get key: " << key << std::endl;
    auto it = data_.find(key);
    if (it != data_.end()) {
        *value = it->second;
        return true;
    }
    return false;
}

void DB::Delete(const std::string& key) {
    std::lock_guard<std::mutex> lock(mutex_);
    std::cout << "[C++] Delete key: " << key << std::endl;
    data_.erase(key);
}

} // namespace lsm
