#pragma once
#include <string>
#include <vector>
#include <memory>
#include <mutex>
#include "core/sstable/table.h"

namespace lsm {

struct FileMetaData {
    int number;
    uint64_t file_size;
    std::string smallest; // Smallest key
    std::string largest;  // Largest key
};

class Version {
public:
    Version(const std::string& dbname);
    ~Version();

    // Add a file to the version
    void AddFile(int level, const FileMetaData& f);
    
    // Look up key in the version's files
    // Returns: 0=NotFound, 1=Found, 2=Deleted
    int Get(const std::string& key, std::string* value);

    std::vector<FileMetaData> GetFiles(int level) const;
    
    void SortL0();

private:
    std::string _dbname;
    // For now, just support Level 0
    std::vector<FileMetaData> _files[7]; // 7 levels
    
    // Simple cache for open tables to avoid re-opening
    // Key: file_number, Value: Table
    // Mutable because Get is const but we might want to cache tables
    mutable std::vector<std::pair<int, std::shared_ptr<Table>>> _table_cache;
    
    std::shared_ptr<Table> GetTable(int file_number);
};

class VersionSet {
public:
    VersionSet(const std::string& dbname);
    ~VersionSet();

    Version* current() const { return _current; }
    
    // Allocate a new file number
    int NewFileNumber() { return _next_file_number++; }
    
    // Apply a change (e.g. add a new SSTable)
    void LogAndApply(Version* edit); // Simplified

    // Recover from disk (scan .sst files)
    void Recover();

private:
    std::string _dbname;
    int _next_file_number;
    Version* _current;
};

} // namespace lsm
