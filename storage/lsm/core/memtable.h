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

private:
    SkipList skiplist_;
};

} // namespace lsm
