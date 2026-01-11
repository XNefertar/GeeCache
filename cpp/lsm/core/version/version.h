#pragma once
#include <string>
#include <vector>
#include <memory>
#include "core/sstable/table.h"
#include "core/version/table_cache.h"

namespace lsm {

struct FileMetaData {
    int number;
    uint64_t file_size;
    std::string smallest; // Smallest key
    std::string largest;  // Largest key
};

class Version {
public:
    explicit Version(TableCache* cache);
    Version(const Version& other);
    ~Version();

    // Add a file to the version
    void AddFile(int level, const FileMetaData& f);
    void RemoveFile(int level, int file_number);
    
    // Look up key in the version's files
    // Returns: 0=NotFound, 1=Found, 2=Deleted
    Table::Status Get(const std::string& key, std::string* value) const;

    std::vector<FileMetaData> GetFiles(int level) const;
    
    void SortL0();

    // Compaction State
    double _compaction_score = -1;
    int _compaction_level = -1;
    
    // Recalculate compaction score
    void Finalize();
    
    std::shared_ptr<Table> GetTable(int file_number) const;

private:
    // std::string _dbname; // Removed
    // For now, just support Level 0
    std::vector<FileMetaData> _files[7]; // 7 levels
    
    TableCache* _table_cache;

    // Optimization: Index files by key for fast lookup
    std::vector<const FileMetaData*> _files_by_key;
    bool _l0_disjoint = false;
};

class VersionSet {
public:
    VersionSet(const std::string& dbname);
    ~VersionSet();
    
    struct Compaction {
        int level;
        std::vector<FileMetaData> inputs[2];
    };

    Version* current() const { return _current; }
    
    // Allocate a new file number
    int NewFileNumber() { return _next_file_number++; }
    
    // Apply a change (e.g. add a new SSTable)
    void LogAndApply(Version* edit); // Simplified

    // Recover from disk (scan .sst files)
    void Recover();
    
    std::unique_ptr<Compaction> PickCompaction();

private:
    std::string _dbname;
    int _next_file_number;
    Version* _current;
    // Owns the TableCache
    std::unique_ptr<TableCache> _table_cache;
};

} // namespace lsm
