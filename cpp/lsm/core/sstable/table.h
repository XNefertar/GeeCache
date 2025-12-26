#pragma once
#include <string>
#include <vector>
#include <fstream>
#include <memory>

namespace lsm {

class Table {
public:
    static std::shared_ptr<Table> Open(const std::string& file_path);
    
    // Returns true if found. value is populated.
    // If deleted, returns true but value is empty (or we need a way to signal deletion).
    // Let's change signature: 
    // Result: 0 = Not Found, 1 = Found, 2 = Deleted
    int Get(const std::string& key, std::string* value);

    class Iterator {
    public:
        Iterator(Table* table);
        bool Valid() const;
        void SeekToFirst();
        void Seek(const std::string& target);
        void Next();
        std::string Key() const;
        std::string Value() const;
        bool IsDeleted() const; // Need to read type
    private:
        Table* _table;
        uint64_t _current_offset;
        // Cache current fields
        std::string _key;
        std::string _value;
        bool _is_deleted;
        bool _valid;
        
        void ParseCurrent();
    };

    Iterator* NewIterator();

private:
    Table(const std::string& file_path);
    bool LoadIndex();

    std::string _file_path;
    std::ifstream _file;
    uint64_t _file_size;
    uint64_t _index_offset;
    
    struct IndexEntry {
        std::string key;
        uint64_t offset;
    };
    std::vector<IndexEntry> _index;
    
    friend class Iterator;
};

} // namespace lsm
