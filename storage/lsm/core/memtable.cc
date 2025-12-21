#include "memtable.h"

namespace lsm {

MemTable::MemTable() {}

void MemTable::Put(const std::string& key, const std::string& value) {
    skiplist_.Insert(key, value, false);
}

bool MemTable::Get(const std::string& key, std::string* value) {
    return skiplist_.Get(key, value);
}

void MemTable::Delete(const std::string& key) {
    skiplist_.Insert(key, "", true);
}

} // namespace lsm
