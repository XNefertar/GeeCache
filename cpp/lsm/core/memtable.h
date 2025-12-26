#pragma once
#include <string>
#include "util/skiplist.h"

namespace lsm {

class MemTable {
public:
    MemTable();
    void Put(const std::string& key, const std::string& value);
    bool Get(const std::string& key, std::string* value);
    void Delete(const std::string& key);

    // Returns a new iterator. The caller must delete it.
    SkipList::Iterator* NewIterator() const;

    size_t MemoryUsage() const { return _skiplist.MemoryUsage(); }

private:
    SkipList _skiplist;
};

} // namespace lsm
