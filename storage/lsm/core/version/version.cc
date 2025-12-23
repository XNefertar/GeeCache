#include "version.h"
#include <algorithm>
#include <iostream>
#include <filesystem>

namespace lsm {

namespace fs = std::filesystem;

Version::Version(const std::string& dbname) : _dbname(dbname) {}
Version::~Version() {}

void Version::AddFile(int level, const FileMetaData& f) {
    _files[level].push_back(f);
}

void Version::SortL0() {
    std::sort(_files[0].begin(), _files[0].end(), [](const FileMetaData& a, const FileMetaData& b) {
        return a.number < b.number;
    });
}

std::shared_ptr<Table> Version::GetTable(int file_number) {
    // Simple linear search in cache
    for (const auto& entry : _table_cache) {
        if (entry.first == file_number) {
            return entry.second;
        }
    }
    
    std::string path = _dbname + "/" + std::to_string(file_number) + ".sst";
    auto table = Table::Open(path);
    if (table) {
        _table_cache.push_back({file_number, table});
    }
    return table;
}

int Version::Get(const std::string& key, std::string* value) {
    // Search L0 files in reverse order (newest first)
    // L0 files can overlap, so we must check all of them that might contain the key
    for (auto it = _files[0].rbegin(); it != _files[0].rend(); ++it) {
        if (key >= it->smallest && key <= it->largest) {
            std::shared_ptr<Table> table = GetTable(it->number);
            if (table) {
                int result = table->Get(key, value);
                if (result != 0) {
                    return result;
                }
            }
        }
    }
    return 0;
}

std::vector<FileMetaData> Version::GetFiles(int level) const {
    if (level < 0 || level >= 7) return {};
    return _files[level];
}

VersionSet::VersionSet(const std::string& dbname) 
    : _dbname(dbname), _next_file_number(1) {
    _current = new Version(dbname);
}

VersionSet::~VersionSet() {
    delete _current;
}

void VersionSet::LogAndApply(Version* edit) {
    // In a real system, we would write to MANIFEST, then update current_.
    // Here we just swap current (leak old one for now or delete if refcounted)
    // Actually, 'edit' here is treated as the new version for simplicity.
    Version* old = _current;
    _current = edit;
    delete old;
}

void VersionSet::Recover() {
    if (!fs::exists(_dbname)) return;

    int max_file_num = 0;

    for (const auto& entry : fs::directory_iterator(_dbname)) {
        if (entry.path().extension() == ".sst") {
            std::string filename = entry.path().filename().string();
            try {
                int file_num = std::stoi(filename.substr(0, filename.find('.')));
                if (file_num > max_file_num) max_file_num = file_num;

                auto table = Table::Open(entry.path().string());
                if (!table) continue;

                FileMetaData meta;
                meta.number = file_num;
                meta.file_size = fs::file_size(entry.path());
                
                auto iter = table->NewIterator();
                iter->SeekToFirst();
                if (iter->Valid()) {
                    meta.smallest = iter->Key();
                    while (iter->Valid()) {
                        meta.largest = iter->Key();
                        iter->Next();
                    }
                }
                delete iter;
                
                _current->AddFile(0, meta);
            } catch (...) {
                continue;
            }
        }
    }
    _next_file_number = max_file_num + 1;
    _current->SortL0();
}

} // namespace lsm
