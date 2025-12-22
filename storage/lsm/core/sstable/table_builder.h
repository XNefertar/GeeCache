#pragma once
#include <string>
#include <vector>
#include <fstream>
#include <cstdint>

namespace lsm {

struct BlockHandle {
    uint64_t offset;
    uint64_t size;
};

class TableBuilder {
public:
    TableBuilder(const std::string& file_path);
    ~TableBuilder();

    void Add(const std::string& key, const std::string& value, bool is_deleted);
    void Finish();
    uint64_t FileSize() const;
    uint64_t NumEntries() const { return _num_entries; }

private:
    std::string _file_path;
    std::ofstream _file;
    uint64_t _offset = 0;
    uint64_t _num_entries = 0;
    
    struct IndexEntry {
        std::string key;
        uint64_t offset;
    };
    std::vector<IndexEntry> _index;
    
    void FlushIndex();
};

} // namespace lsm
