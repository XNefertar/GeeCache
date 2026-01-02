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
    
    _keys.push_back(key);
    _last_key = key;

    uint32_t klen = key.size();
    uint32_t vlen = value.size();
    uint8_t type = is_deleted ? 1 : 0;

    _current_block.append(reinterpret_cast<const char*>(&klen), sizeof(klen));
    _current_block.append(key);
    _current_block.append(reinterpret_cast<const char*>(&vlen), sizeof(vlen));
    _current_block.append(value);
    _current_block.append(reinterpret_cast<const char*>(&type), sizeof(type));

    _num_entries++;

    if (_current_block.size() >= 4096) {
        FlushBlock();
    }
}

void TableBuilder::FlushBlock() {
    if (_current_block.empty()) return;
    
    uint64_t block_size = _current_block.size();
    _file.write(_current_block.data(), block_size);
    
    // Index points to the end key of the block
    _index.push_back({_last_key, _offset, block_size});
    
    _offset += block_size;
    _current_block.clear();
}

void TableBuilder::Finish() {
    if (!_file.is_open()) return;

    // Flush remaining data
    FlushBlock();

    // 1. Generate and Write Bloom Filter
    BloomFilterPolicy filter_policy;
    std::string filter_data;
    filter_policy.CreateFilter(_keys, &filter_data);

    uint64_t filter_offset = _offset;
    uint64_t filter_size = filter_data.size();
    _file.write(filter_data.data(), filter_size);
    _offset += filter_size;

    // 2. Write Index
    uint64_t index_offset = _offset;
    uint32_t index_size = _index.size();
    
    _file.write(reinterpret_cast<const char*>(&index_size), sizeof(index_size));
    for (const auto& entry : _index) {
        uint32_t klen = entry.key.size();
        _file.write(reinterpret_cast<const char*>(&klen), sizeof(klen));
        _file.write(entry.key.data(), klen);
        _file.write(reinterpret_cast<const char*>(&entry.offset), sizeof(entry.offset));
        _file.write(reinterpret_cast<const char*>(&entry.size), sizeof(entry.size));
    }
    
    // 3. Write Footer
    _file.write(reinterpret_cast<const char*>(&index_offset), sizeof(index_offset));
    _file.write(reinterpret_cast<const char*>(&filter_offset), sizeof(filter_offset));
    _file.write(reinterpret_cast<const char*>(&filter_size), sizeof(filter_size));
    
    _file.flush();
    _file.close();
}



uint64_t TableBuilder::FileSize() const {
    return _offset;
}

} // namespace lsm
