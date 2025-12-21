#pragma once
#include <string>
#include <mutex>
#include <memory>
#include <thread>
#include <atomic>
#include "memtable.h"
#include "wal.h"

namespace lsm {

struct Options {
    bool sync = false; // true: fsync on every write, false: rely on background sync
};

class DB {
public:
    DB(const std::string& path, const Options& options = Options());
    ~DB();

    void Put(const std::string& key, const std::string& value);
    bool Get(const std::string& key, std::string* value);
    void Delete(const std::string& key);

private:
    std::string path_;
    Options options_;
    std::unique_ptr<MemTable> memtable_;
    std::unique_ptr<WAL> wal_;
    std::mutex mutex_;
    
    std::thread sync_thread_;
    std::atomic<bool> stop_sync_;
    void BackgroundSync();
    
    void Recover(const std::string& wal_path);
};

} // namespace lsm
