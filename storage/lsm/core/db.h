#pragma once
#include <string>
#include <mutex>
#include <memory>
#include <thread>
#include <atomic>
#include "memtable.h"
#include "wal.h"
#include "core/version/version.h"

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
    std::string _path;
    Options _options;
    std::unique_ptr<MemTable> _memtable;
    std::unique_ptr<WAL> _wal;
    std::unique_ptr<VersionSet> _versions;
    std::mutex _mutex;
    
    std::thread _sync_thread;
    std::atomic<bool> _stop_sync;
    void BackgroundSync();
    
    void Recover(const std::string& wal_path);
};

} // namespace lsm
