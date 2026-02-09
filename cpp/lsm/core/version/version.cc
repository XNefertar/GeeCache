#include "version.h"
#include "core/sstable/table.h"
#include "core/coding.h"
#include <algorithm>
#include <iostream>
#include <filesystem>
#include <chrono>
#include <cmath>

namespace lsm {

namespace fs = std::filesystem;

Version::Version(TableCache* cache) : _table_cache(cache) {}

Version::Version(const Version& other) 
    : _table_cache(other._table_cache), // Copy the pointer
      _compaction_score(other._compaction_score), 
      _compaction_level(other._compaction_level),
      _l0_disjoint(other._l0_disjoint),
      _files_by_key(other._files_by_key)
{
    for (int i=0; i<7; ++i) {
        _files[i] = other._files[i];
    }
}

Version::~Version() {}

void Version::AddFile(int level, const FileMetaData& f) {
    _files[level].push_back(f);
}

void Version::RemoveFile(int level, int file_number) {
    auto& files = _files[level];
    files.erase(std::remove_if(files.begin(), files.end(), 
        [file_number](const FileMetaData& f) { return f.number == file_number; }), 
        files.end());
}

void Version::SortL0() {
    std::sort(_files[0].begin(), _files[0].end(), [](const FileMetaData& a, const FileMetaData& b) {
        return a.number < b.number;
    });

    // Build index
    _files_by_key.clear();
    for (const auto& f : _files[0]) {
        _files_by_key.push_back(&f);
    }
    std::sort(_files_by_key.begin(), _files_by_key.end(), [](const FileMetaData* a, const FileMetaData* b) {
        return a->smallest < b->smallest;
    });
    
    // Check disjointness
    _l0_disjoint = true;
    if (!_files_by_key.empty()) {
        for (size_t i = 0; i < _files_by_key.size() - 1; ++i) {
            if (_files_by_key[i]->largest >= _files_by_key[i+1]->smallest) {
                _l0_disjoint = false;
                break;
            }
        }
    }
}

void Version::SortLevel(int level) {
    if (level == 0) {
        SortL0();
        return;
    }
    if (level < 0 || level >= 7) return;
    std::sort(_files[level].begin(), _files[level].end(), [](const FileMetaData& a, const FileMetaData& b) {
        return a.smallest < b.smallest;
    });
}

std::shared_ptr<Table> Version::GetTable(int file_number) const {
    if (_table_cache) {
        return _table_cache->FindTable(file_number);
    }
    return nullptr;
}

Table::Status Version::Get(const std::string& key, std::string* value) const {
    auto in_range = [](const std::string& k, const FileMetaData* f) {
        if (k > f->largest) return false;
        if (k >= f->smallest) return true;
        return CodingUtil::ExtractUserKey(k) == CodingUtil::ExtractUserKey(f->smallest);
    };
    // Fast path for disjoint files
    if (_l0_disjoint) {
        auto it = std::upper_bound(_files_by_key.begin(), _files_by_key.end(), key, 
            [](const std::string& k, const FileMetaData* f) {
                return k < f->smallest;
            });
            
        // Check the file that is strictly "greater" than key (because valid UserKey might appear "greater")
        if (it != _files_by_key.end()) {
            const FileMetaData* f = *it;
            if (in_range(key, f)) {
                std::shared_ptr<Table> table = GetTable(f->number);
                if (table) {
                     Table::Status status = table->Get(key, value);
                     if (status != Table::kNotFound) return status;
                }
            }
        }

        if (it != _files_by_key.begin()) {
            const FileMetaData* f = *(--it);
            if (in_range(key, f)) {
                std::shared_ptr<Table> table = GetTable(f->number);
                if (table) { // Safety check
                    Table::Status status = table->Get(key, value);
                    if (status != Table::kNotFound) {
                        return status;
                    }
                }
            }
        }
    } else {
        // Search L0 files in reverse order (newest first)
        // L0 files can overlap, so we must check all of them that might contain the key
        for (auto it = _files[0].rbegin(); it != _files[0].rend(); ++it) {
            if (in_range(key, &(*it))) {
                std::shared_ptr<Table> table = GetTable(it->number);
                if (table) {
                    Table::Status status = table->Get(key, value);
                    if (status != Table::kNotFound) {
                        return status;
                    }
                }
            }
        }
    }
    
    for (int level = 1; level < 7; ++level) {
        const auto& files = _files[level];
        auto it = std::lower_bound(files.begin(), files.end(), key,
            [](const FileMetaData& f, const std::string& k) {
                return f.largest < k;
            });
        if (it != files.end() && in_range(key, &(*it))) {
            std::shared_ptr<Table> table = GetTable(it->number);
            if (table) {
                Table::Status status = table->Get(key, value);
                if (status != Table::kNotFound) {
                    return status;
                }
            }
        }
    }
    return Table::kNotFound;
}

std::vector<FileMetaData> Version::GetFiles(int level) const {
    if (level < 0 || level >= 7) return {};
    return _files[level];
}

VersionSet::VersionSet(const std::string& dbname) 
    : _dbname(dbname), _next_file_number(1) {
    _table_cache = std::make_unique<TableCache>(dbname);
    _current = new Version(_table_cache.get());
}

VersionSet::~VersionSet() {
    delete _current;
}

void VersionSet::LogAndApply(Version* edit) {
    edit->SortL0(); // Rebuild index (important because copy ctor copies pointers to old files)
    edit->Finalize();
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
    _current->Finalize();
}

void Version::Finalize() {
    int best_level = -1;
    double best_score = -1;

    // Level 0: Score = num_files / 4.0
    {
        double score = _files[0].size() / 4.0;
        if (score > best_score) {
            best_score = score;
            best_level = 0;
        }
    }

    // Level 1-6
    for (int level = 1; level < 7; ++level) {
        // Target size for Level L = 10MB * 10^(L-1)
        double target_size = 10.0 * 1024.0 * 1024.0 * std::pow(10.0, level - 1);
        
        uint64_t level_size = 0;
        for (const auto& f : _files[level]) {
            level_size += f.file_size;
        }
        
        double score = level_size / target_size;
        if (score > best_score) {
            best_score = score;
            best_level = level;
        }
    }

    _compaction_level = best_level;
    _compaction_score = best_score;
}

std::unique_ptr<VersionSet::Compaction> VersionSet::PickCompaction() {
    _current->Finalize();
    
    if (_current->_compaction_score < 1.0) {
        return nullptr;
    }
    
    auto c = std::make_unique<Compaction>();
    c->level = _current->_compaction_level;
    
    // Setup inputs[0]
    if (c->level == 0) {
        // Compact all L0 files
        c->inputs[0] = _current->GetFiles(0);
    } else {
        // Pick the first file from level
        const auto& files = _current->GetFiles(c->level);
        if (!files.empty()) {
             c->inputs[0].push_back(files[0]);
        }
    }
    
    if (c->inputs[0].empty()) return nullptr;
    
    // Setup inputs[1] (overlapping files in level+1)
    if (c->level + 1 < 7) {
        std::string smallest = c->inputs[0][0].smallest;
        std::string largest = c->inputs[0][0].largest;
        for (size_t i = 1; i < c->inputs[0].size(); ++i) {
            if (c->inputs[0][i].smallest < smallest) smallest = c->inputs[0][i].smallest;
            if (c->inputs[0][i].largest > largest) largest = c->inputs[0][i].largest;
        }
        
        const auto& next_files = _current->GetFiles(c->level + 1);
        for (const auto& f : next_files) {
            if (f.largest >= smallest && f.smallest <= largest) {
                c->inputs[1].push_back(f);
            }
        }
    }
    
    return c;
}

} // namespace lsm
