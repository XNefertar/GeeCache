#include "table.h"
#include <iostream>
#include <algorithm>

namespace lsm {

std::shared_ptr<Table> Table::Open(const std::string& file_path) {
    auto table = std::shared_ptr<Table>(new Table(file_path));
    if (table->LoadIndex()) {
        return table;
    }
    return nullptr;
}

Table::Table(const std::string& file_path) : _file_path(file_path) {
    _file.open(file_path, std::ios::binary | std::ios::ate);
    _file_size = _file.tellg();
}

bool Table::LoadIndex() {
    if (!_file.is_open() || _file_size < 8) return false;

    // Read Footer
    _file.seekg(_file_size - 8);
    _file.read(reinterpret_cast<char*>(&_index_offset), 8);

    if (_index_offset >= _file_size) return false;

    // Read Index
    _file.seekg(_index_offset);
    uint32_t index_size;
    _file.read(reinterpret_cast<char*>(&index_size), sizeof(index_size));

    for (uint32_t i = 0; i < index_size; ++i) {
        uint32_t klen;
        _file.read(reinterpret_cast<char*>(&klen), sizeof(klen));
        std::string key(klen, '\0');
        _file.read(&key[0], klen);
        uint64_t offset;
        _file.read(reinterpret_cast<char*>(&offset), sizeof(offset));
        
        _index.push_back({key, offset});
    }
    return true;
}

int Table::Get(const std::string& key, std::string* value) {
    // Binary search in index
    auto it = std::lower_bound(_index.begin(), _index.end(), key, 
        [](const IndexEntry& entry, const std::string& k) {
            return entry.key < k;
        });

    if (it != _index.end() && it->key == key) {
        // Found exact match in index (since we index every key in this simple version)
        // Read from file
        _file.seekg(it->offset);
        
        uint32_t klen, vlen;
        uint8_t type;
        
        _file.read(reinterpret_cast<char*>(&klen), sizeof(klen));
        _file.seekg(klen, std::ios::cur); // Skip key
        
        _file.read(reinterpret_cast<char*>(&vlen), sizeof(vlen));
        std::string val(vlen, '\0');
        _file.read(&val[0], vlen);
        
        _file.read(reinterpret_cast<char*>(&type), sizeof(type));
        
        if (type == 1) return 2; // Deleted
        *value = val;
        return 1; // Found
    }
    
    return 0; // Not found
}

Table::Iterator* Table::NewIterator() {
    return new Iterator(this);
}

// Iterator Implementation
Table::Iterator::Iterator(Table* table) : _table(table), _current_offset(0), _valid(false) {}

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
    
    _table->_file.seekg(_current_offset);
    
    uint32_t klen;
    _table->_file.read(reinterpret_cast<char*>(&klen), sizeof(klen));
    _key.resize(klen);
    _table->_file.read(&_key[0], klen);
    
    uint32_t vlen;
    _table->_file.read(reinterpret_cast<char*>(&vlen), sizeof(vlen));
    _value.resize(vlen);
    _table->_file.read(&_value[0], vlen);
    
    uint8_t type;
    _table->_file.read(reinterpret_cast<char*>(&type), sizeof(type));
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
