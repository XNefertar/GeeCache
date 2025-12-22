#include "table_builder.h"
#include <iostream>

namespace lsm {

TableBuilder::TableBuilder(const std::string& file_path) 
    : _file_path(file_path) {
    _file.open(file_path, std::ios::binary | std::ios::trunc);
}

TableBuilder::~TableBuilder() {
    if (_file.is_open()) {
        _file.close();
    }
}

void TableBuilder::Add(const std::string& key, const std::string& value, bool is_deleted) {
    if (!_file.is_open()) return;
    
    _index.push_back({key, _offset});

    uint32_t klen = key.size();
    uint32_t vlen = value.size();
    uint8_t type = is_deleted ? 1 : 0;

    _file.write(reinterpret_cast<const char*>(&klen), sizeof(klen));
    _file.write(key.data(), klen);
    _file.write(reinterpret_cast<const char*>(&vlen), sizeof(vlen));
    _file.write(value.data(), vlen);
    _file.write(reinterpret_cast<const char*>(&type), sizeof(type));

    _offset += sizeof(klen) + klen + sizeof(vlen) + vlen + sizeof(type);
    _num_entries++;
}

void TableBuilder::Finish() {
    if (!_file.is_open()) return;

    // Write Index
    uint64_t index_offset = _offset;
    uint32_t index_size = _index.size();
    
    _file.write(reinterpret_cast<const char*>(&index_size), sizeof(index_size));
    for (const auto& entry : _index) {
        uint32_t klen = entry.key.size();
        _file.write(reinterpret_cast<const char*>(&klen), sizeof(klen));
        _file.write(entry.key.data(), klen);
        _file.write(reinterpret_cast<const char*>(&entry.offset), sizeof(entry.offset));
    }

    // Write Footer: Index Offset
    _file.write(reinterpret_cast<const char*>(&index_offset), sizeof(index_offset));
    
    _file.flush();
    _file.close();
}

uint64_t TableBuilder::FileSize() const {
    return _offset;
}

} // namespace lsm
