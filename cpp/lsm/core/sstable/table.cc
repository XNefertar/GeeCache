#include "table.h"
#include <iostream>
#include <algorithm>
#include <cstring>
#include <atomic>
#include <chrono>
#include <sys/mman.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <unistd.h>

namespace lsm {

std::shared_ptr<Table> Table::Open(const std::string& file_path) {
    auto table = std::shared_ptr<Table>(new Table(file_path));
    if (table->LoadIndex()) {
        return table;
    }
    return nullptr;
}

Table::Table(const std::string& file_path) : _file_path(file_path) {
    _fd = open(file_path.c_str(), O_RDONLY);
    if (_fd != -1) {
        struct stat sb;
        if (fstat(_fd, &sb) != -1) {
            _file_size = sb.st_size;
            _mapped_data = (char*)mmap(nullptr, _file_size, PROT_READ, MAP_PRIVATE, _fd, 0);
            if (_mapped_data == MAP_FAILED) {
                _mapped_data = nullptr;
                _file_size = 0;
            }
        }
    }
}

Table::~Table() {
    if (_mapped_data) {
        munmap(_mapped_data, _file_size);
    }
    if (_fd != -1) {
        close(_fd);
    }
}

bool Table::LoadIndex() {
    if (!_mapped_data || _file_size < 24) return false;

    // Read Footer (24 bytes)
    // _file.seekg(_file_size - 24);
    const char* footer = _mapped_data + _file_size - 24;
    
    uint64_t filter_offset, filter_size;
    memcpy(&_index_offset, footer, 8);
    memcpy(&filter_offset, footer + 8, 8);
    memcpy(&filter_size, footer + 16, 8);

    if (_index_offset >= _file_size) return false;

    // Read Bloom Filter
    if (filter_size > 0) {
        _filter_data.assign(_mapped_data + filter_offset, filter_size);
    }

    // Read Index
    const char* index_ptr = _mapped_data + _index_offset;
    uint32_t index_size;
    memcpy(&index_size, index_ptr, sizeof(index_size));
    index_ptr += sizeof(index_size);

    for (uint32_t i = 0; i < index_size; ++i) {
        uint32_t klen;
        memcpy(&klen, index_ptr, sizeof(klen));
        index_ptr += sizeof(klen);
        
        std::string key(index_ptr, klen);
        index_ptr += klen;
        
        uint64_t offset, size;
        memcpy(&offset, index_ptr, sizeof(offset));
        index_ptr += sizeof(offset);
        memcpy(&size, index_ptr, sizeof(size));
        index_ptr += sizeof(size);
        
        _index.push_back({key, offset, size});
    }
    return true;
}

Table::Status Table::Get(const std::string& key, std::string* value) {
    // Check Bloom Filter first
    if (!_filter_data.empty() && !_filter_policy.KeyMayMatch(key, _filter_data)) {
        return kNotFound; // Definitely not found
    }

    // Binary search in index
    auto it = std::lower_bound(_index.begin(), _index.end(), key, 
        [](const IndexEntry& entry, const std::string& k) {
            return entry.key < k;
        });

    if (it == _index.end()) {
        return kNotFound;
    }

    // Read Block (Directly from mmap)
    const char* block_data = _mapped_data + it->offset;
    size_t block_size = it->size;
    
    // Scan Block
    const char* data = block_data;
    const char* end = data + block_size;
    
    while (data < end) {
        uint32_t klen;
        memcpy(&klen, data, sizeof(klen));
        data += sizeof(klen);
        
        // Optimization: Compare key without allocation
        if (klen == key.size() && memcmp(data, key.data(), klen) == 0) {
            data += klen; // Skip key
            
            uint32_t vlen;
            memcpy(&vlen, data, sizeof(vlen));
            data += sizeof(vlen);
            
            *value = std::string(data, vlen);
            data += vlen;
            
            uint8_t type;
            memcpy(&type, data, sizeof(type));
            
            if (type == 1) return kDeleted; // Deleted
            return kFound; // Found
        }
        
        // Skip key
        data += klen;
        
        // Skip value
        uint32_t vlen;
        memcpy(&vlen, data, sizeof(vlen));
        data += sizeof(vlen);
        data += vlen;
        
        // Skip type
        data += 1;
    }
    
    return kNotFound; // Not found
}


Table::Iterator* Table::NewIterator() {
    return new Iterator(shared_from_this());
}

// Iterator Implementation
Table::Iterator::Iterator(std::shared_ptr<Table> table) : _table(std::move(table)), _current_offset(0), _valid(false) {}

bool Table::Iterator::Valid() const {
    return _valid;
}

void Table::Iterator::SeekToFirst() {
    _current_offset = 0;
    if (_table->_index_offset > 0) {
        ParseCurrent();
    } else {
        _valid = false;
    }
}

void Table::Iterator::Seek(const std::string& target) {
    auto it = std::lower_bound(_table->_index.begin(), _table->_index.end(), target, 
        [](const IndexEntry& entry, const std::string& k) {
            return entry.key < k;
        });

    if (it != _table->_index.end()) {
        _current_offset = it->offset;
        ParseCurrent();
    } else {
        _valid = false;
    }
}

void Table::Iterator::Next() {
    if (!_valid) return;
    // Move offset past current entry
    // Current entry size: 4 + klen + 4 + vlen + 1
    uint32_t klen = _key.size();
    uint32_t vlen = _value.size();
    _current_offset += 4 + klen + 4 + vlen + 1;
    
    if (_current_offset >= _table->_index_offset) {
        _valid = false;
    } else {
        ParseCurrent();
    }
}

void Table::Iterator::ParseCurrent() {
    if (_current_offset >= _table->_index_offset) {
        _valid = false;
        return;
    }
    
    const char* ptr = _table->_mapped_data + _current_offset;
    
    uint32_t klen;
    memcpy(&klen, ptr, sizeof(klen));
    ptr += sizeof(klen);
    
    _key.assign(ptr, klen);
    ptr += klen;
    
    uint32_t vlen;
    memcpy(&vlen, ptr, sizeof(vlen));
    ptr += sizeof(vlen);
    
    _value.assign(ptr, vlen);
    ptr += vlen;
    
    uint8_t type;
    memcpy(&type, ptr, sizeof(type));
    _is_deleted = (type == 1);
    
    _valid = true;
}

std::string Table::Iterator::Key() const {
    return _key;
}

std::string Table::Iterator::Value() const {
    return _value;
}

bool Table::Iterator::IsDeleted() const {
    return _is_deleted;
}

} // namespace lsm
