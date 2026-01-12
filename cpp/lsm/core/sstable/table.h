#pragma once
#include <string>
#include <vector>
#include <fstream>
#include <memory>
#include "util/bloom_filter.h"
#include "core/iterator.h"

namespace lsm {

class Table : public std::enable_shared_from_this<Table> {
public:

    enum Status {
        kNotFound = 0,
        kFound = 1,
        kDeleted = 2
    };

    static std::shared_ptr<Table> Open(const std::string& file_path);
    ~Table(); // Need destructor to unmap
    
    // Returns true if found. value is populated.
    // If deleted, returns true but value is empty (or we need a way to signal deletion).
    // Let's change signature: 
    // Result: 0 = Not Found, 1 = Found, 2 = Deleted
    Status Get(const std::string& key, std::string* value);

    class Iterator : public lsm::Iterator {
    public:
        Iterator(std::shared_ptr<Table> table);
        // Implement lsm::Iterator
        bool Valid() const override;
        void SeekToFirst() override;
        void Seek(const std::string& target) override;
        void Next() override;
        std::string Key() const override;
        std::string Value() const override;
        bool IsDeleted() const override;
    private:
        std::shared_ptr<Table> _table;
        uint64_t _current_offset;
        // Cache current fields
        std::string _key;
        std::string _value;
        bool _is_deleted{false};
        bool _valid{false};
        
        void ParseCurrent();
    };

    Iterator* NewIterator();

private:
    Table(const std::string& file_path);
    bool LoadIndex();

    std::string _file_path;
    // std::ifstream _file; // Removed
    int _fd = -1;
    char* _mapped_data = nullptr;
    uint64_t _file_size = 0;
    uint64_t _index_offset;
    
    struct IndexEntry {
        std::string key;
        uint64_t offset;
        uint64_t size;
    };

    std::vector<IndexEntry> _index;
    
    std::string _filter_data;
    BloomFilterPolicy _filter_policy;

    // Simple 1-block cache
    uint64_t _last_block_offset = (uint64_t)-1;
    std::string _last_block_data;

    friend class Iterator;
};


} // namespace lsm
