#pragma once
#include <string>
#include <mutex>
#include <memory>
#include "memtable.h"
#include "wal.h"

namespace lsm {

class DB {
public:
    DB(const std::string& path);
    ~DB();

    void Put(const std::string& key, const std::string& value);
    bool Get(const std::string& key, std::string* value);
    void Delete(const std::string& key);

private:
    std::string path_;
    std::unique_ptr<MemTable> memtable_;
    std::unique_ptr<WAL> wal_;
    std::mutex mutex_;
    
    void Recover(const std::string& wal_path);
};

} // namespace lsm
