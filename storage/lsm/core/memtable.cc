#include "memtable.h"

namespace lsm {

MemTable::MemTable() {}

void MemTable::Put(const std::string& key, const std::string& value) {
    _skiplist.Insert(key, value, false);
}

bool MemTable::Get(const std::string& key, std::string* value) {
    return _skiplist.Get(key, value);
}

void MemTable::Delete(const std::string& key) {
    _skiplist.Insert(key, "", true);
}

SkipList::Iterator* MemTable::NewIterator() const {
    return _skiplist.NewIterator();
}

} // namespace lsm
