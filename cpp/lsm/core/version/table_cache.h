#pragma once
#include <string>
#include <memory>
#include <mutex>
#include <unordered_map>
#include "core/sstable/table.h"

namespace lsm {

class TableCache {
public:
    TableCache(const std::string& dbname);
    
    // Non-const because it modifies the cache
    std::shared_ptr<Table> FindTable(int file_number);
    
    // Evict a table from cache
    void Evict(int file_number);

private:
    std::string _dbname;
    std::mutex _mutex;
    // Simple cache for open tables
    std::unordered_map<int, std::shared_ptr<Table>> _cache;
};

} // namespace lsm
