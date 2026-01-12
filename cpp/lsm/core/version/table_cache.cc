#include "table_cache.h"
#include <iostream>

namespace lsm {

TableCache::TableCache(const std::string& dbname) : _dbname(dbname) {}

std::shared_ptr<Table> TableCache::FindTable(int file_number) {
    std::lock_guard<std::mutex> lock(_mutex);
    
    auto it = _cache.find(file_number);
    if (it != _cache.end()) {
        if (it->second) {
            return it->second;
        }
        // Defensive: Remove stale nullptr from cache so we can retry open
        _cache.erase(it);
    }
    
    std::string path = _dbname + "/" + std::to_string(file_number) + ".sst";
    auto table = Table::Open(path);
    if (table) {
        _cache[file_number] = table;
    } else {
        std::cerr << "TableCache failed to open: " << path << std::endl;
        // Do not cache nullptr to allow retrying later
    }
    return table;
}

void TableCache::Evict(int file_number) {
    std::lock_guard<std::mutex> lock(_mutex);
    _cache.erase(file_number);
}

} // namespace lsm
