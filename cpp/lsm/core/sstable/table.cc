#include "table.h"
#include "core/coding.h"
#include <string_view>
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
    const char* footer = _mapped_data + _file_size - 24;
    
    uint64_t filter_offset, filter_size;
    memcpy(&_index_offset, footer, 8);
    memcpy(&filter_offset, footer + 8, 8);
    memcpy(&filter_size, footer + 16, 8);

    if (_index_offset >= _file_size) return false;

    // Read Bloom Filter
    if (filter_size > 0 && filter_offset + filter_size <= _file_size) {
        _filter_data.assign(_mapped_data + filter_offset, filter_size);
    }

    // Read Index
    const char* index_ptr = _mapped_data + _index_offset;
    if (index_ptr + sizeof(uint32_t) > _mapped_data + _file_size) return false;

    uint32_t index_size;
    memcpy(&index_size, index_ptr, sizeof(index_size));
    index_ptr += sizeof(index_size);

    for (uint32_t i = 0; i < index_size; ++i) {
        if (index_ptr + sizeof(uint32_t) > _mapped_data + _file_size) break;
        uint32_t klen;
        memcpy(&klen, index_ptr, sizeof(klen));
        index_ptr += sizeof(klen);
        
        if (index_ptr + klen > _mapped_data + _file_size) break;
        std::string key(index_ptr, klen);
        index_ptr += klen;
        
        if (index_ptr + 16 > _mapped_data + _file_size) break;
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
    // Check Bloom Filter first (Check User Key)
    if (!_filter_data.empty() && !_filter_policy.KeyMayMatch(CodingUtil::ExtractUserKey(key), _filter_data)) {
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

    // Check Block Bounds
    if (it->offset + it->size > _file_size) {
        std::cerr << "[Error] Block out of file bounds!" << std::endl;
        return kNotFound;
    }
    
    const char* block_data = _mapped_data + it->offset;
    const char* data = block_data;
    const char* end = block_data + it->size;
    
    while (data < end) {
        // Check bounds
        if (data + sizeof(uint32_t) > end) break;
        uint32_t klen;
        memcpy(&klen, data, sizeof(klen));
        data += sizeof(klen);
        
        if (data + klen > end) {
             std::cerr << "[Error] Block Corruption: Key length " << klen << " exceeds block bounds." << std::endl;
             std::cerr << "Offset in block: " << (data - block_data) << ", Block Size: " << it->size << std::endl;
             break;
        }
        std::string_view current_key(data, klen);
        
        if (current_key >= key) {
             std::string curr_str(current_key);
             if (CodingUtil::ExtractUserKey(curr_str) == CodingUtil::ExtractUserKey(key)) {
                
                // FOUND Matching User Key.
                // Since data is sorted by Internal Key (Desc Seq), the first one we see is the valid one.
                
                // Check bounds for Value
                const char* v_ptr = data + klen;
                if (v_ptr + sizeof(uint32_t) > end) {
                    std::cerr << "[Error] Block Corruption: Value length header out of bounds" << std::endl;
                    return kNotFound;
                }

                uint32_t vlen;
                memcpy(&vlen, v_ptr, sizeof(vlen));
                v_ptr += sizeof(vlen);
                
                if (v_ptr + vlen + 1 > end) { // +1 for type
                     std::cerr << "[Error] Block Corruption: Value/Type out of bounds" << std::endl;
                     return kNotFound;
                }

                *value = std::string(v_ptr, vlen);
                uint8_t type;
                memcpy(&type, v_ptr + vlen, sizeof(type));
                
                if (type == 1) return kDeleted;
                return kFound;
             } else {
                 return kNotFound;
             }
        }
        
        // Skip current entry to move to next
        const char* next_entry = data + klen;
        if (next_entry + sizeof(uint32_t) > end) break;
        
        uint32_t vlen;
        memcpy(&vlen, next_entry, sizeof(vlen));
        next_entry += sizeof(vlen);
        
        if (next_entry + vlen + 1 > end) break;
        next_entry += vlen + 1; // +1 for type
        
        data = next_entry;
    }
    
    return kNotFound; // Not found
}

Table::Iterator* Table::NewIterator() {
    return new Iterator(shared_from_this());
}

Table::Iterator::Iterator(std::shared_ptr<Table> table) 
    : _table(std::move(table)), _current_offset(0), _valid(false) {
    if (_table->_index_offset > 0) {
        ParseCurrent();
    }
}

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
        
        // Linear scan in block to find key >= target
        while (_valid && _key < target) {
            Next();
        }
    } else {
        _valid = false;
    }
}

void Table::Iterator::Next() {
    if (!_valid) return;
    // Current entry size: 4(klen) + klen + 4(vlen) + vlen + 1(type)
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
    // Check global bounds
    if (_current_offset >= _table->_index_offset) {
        _valid = false;
        return;
    }
    
    const char* ptr = _table->_mapped_data + _current_offset;
    
    // Safety Checks
    if (_current_offset + 4 > _table->_index_offset) { _valid = false; return; }
    uint32_t klen;
    memcpy(&klen, ptr, sizeof(klen));
    ptr += sizeof(klen);
    
    if (_current_offset + 4 + klen > _table->_index_offset) { _valid = false; return; }
    _key.assign(ptr, klen);
    ptr += klen;
    
    if (_current_offset + 4 + klen + 4 > _table->_index_offset) { _valid = false; return; }
    uint32_t vlen;
    memcpy(&vlen, ptr, sizeof(vlen));
    ptr += sizeof(vlen);
    
    if (_current_offset + 4 + klen + 4 + vlen + 1 > _table->_index_offset) { _valid = false; return; }
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

