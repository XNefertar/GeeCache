#include "db.h"
#include <iostream>
#include <filesystem>
#include <chrono>
#include <fstream>
#include "core/sstable/table_builder.h"
#include "core/sstable/table.h"
#include "util/merging_iterator.h"

namespace lsm {

namespace fs = std::filesystem;

DB::DB(const std::string& path, const Options& options) 
    : _path(path), _options(options), _stop_sync(false), _stop_compaction(false) {
    if (!fs::exists(path)) {
        fs::create_directories(path);
    }
    
    std::string wal_path = path + "/wal.log";
    
    _memtable = std::make_unique<MemTable>();
    _versions = std::make_unique<VersionSet>(path);
    _versions->Recover();
    
    Recover(wal_path);
    
    _wal = std::make_unique<WAL>(wal_path);
    
    if (!_options.sync) {
        _sync_thread = std::thread(&DB::BackgroundSync, this);
    }

    _compaction_thread = std::thread(&DB::BackgroundCompaction, this);
    
    std::cout << "[C++] DB opened at " << _path << std::endl;
}

DB::~DB() {
    _stop_sync = true;
    _stop_compaction = true;
    _compaction_cv.notify_all();

    if (_sync_thread.joinable()) {
        _sync_thread.join();
    }
    if (_compaction_thread.joinable()) {
        _compaction_thread.join();
    }

    // Force final sync on close
    if (_wal) {
        _wal->Sync();
    }
    std::cout << "[C++] DB closed" << std::endl;
}

void DB::Put(const std::string& key, const std::string& value) {
    std::lock_guard<std::mutex> lock(_mutex);
    
    if (_memtable->MemoryUsage() >= kMemTableSizeLimit) {
        Flush();
    }

    _wal->Append(key, value, false);
    if (_options.sync) {
        _wal->Sync();
    }
    _memtable->Put(key, value);
}

bool DB::Get(const std::string& key, std::string* value) {
    std::lock_guard<std::mutex> lock(_mutex);
    if (_memtable->Get(key, value)) {
        return true;
    }
    // Check SSTables via Version
    Table::Status result = _versions->current()->Get(key, value);
    if (result == Table::kFound) return true; // Found
    if (result == Table::kDeleted) return false; // Deleted
    return Table::kNotFound; // Not found
}

void DB::Delete(const std::string& key) {
    std::lock_guard<std::mutex> lock(_mutex);
    
    if (_memtable->MemoryUsage() >= kMemTableSizeLimit) {
        Flush();
    }

    _wal->Append(key, "", true);
    if (_options.sync) {
        _wal->Sync();
    }
    _memtable->Delete(key);
}

void DB::Flush() {
    if (_memtable->MemoryUsage() == 0) return;

    int file_num = _versions->NewFileNumber();
    std::string fname = _path + "/" + std::to_string(file_num) + ".sst";
    TableBuilder builder(fname);

    auto iter = _memtable->NewIterator();
    iter->SeekToFirst();
    
    if (!iter->Valid()) {
        delete iter;
        return;
    }

    std::string smallest = iter->Key();
    std::string largest;

    while (iter->Valid()) {
        largest = iter->Key();
        builder.Add(iter->Key(), iter->Value(), iter->IsDeleted());
        iter->Next();
    }
    delete iter;

    builder.Finish();

    FileMetaData meta;
    meta.number = file_num;
    meta.file_size = builder.FileSize();
    meta.smallest = smallest;
    meta.largest = largest;

    Version* new_version = new Version(*_versions->current());
    new_version->AddFile(0, meta);
    _versions->LogAndApply(new_version);

    // Reset MemTable and WAL
    _memtable = std::make_unique<MemTable>();
    
    // Close old WAL
    _wal.reset();
    // Remove old WAL file
    std::string wal_path = _path + "/wal.log";
    fs::remove(wal_path);
    // Create new WAL
    _wal = std::make_unique<WAL>(wal_path);
    
    std::cout << "[C++] Flushed MemTable to " << fname << std::endl;
    
    MaybeScheduleCompaction();
}

void DB::BackgroundSync() {
    while (!_stop_sync) {
        std::this_thread::sleep_for(std::chrono::seconds(1));
        if (_stop_sync) break;
        if (_wal) {
            _wal->Sync();
        }
    }
}

void DB::Recover(const std::string& wal_path) {
    if (!fs::exists(wal_path)) return;

    std::ifstream file(wal_path, std::ios::binary);
    if (!file.is_open()) return;

    std::streampos valid_pos = 0;

    while (file.peek() != EOF) {
        char type;
        uint32_t klen;
        
        // Read header
        file.read(&type, 1);
        if (file.gcount() != 1) break;
        
        file.read(reinterpret_cast<char*>(&klen), sizeof(klen));
        if (file.gcount() != sizeof(klen)) break;
        
        // Read key
        std::string key(klen, '\0');
        file.read(&key[0], klen);
        if (file.gcount() != klen) break;
        
        // Read value (if not delete)
        std::string value;
        if (type == 0) {
            uint32_t vlen;
            file.read(reinterpret_cast<char*>(&vlen), sizeof(vlen));
            if (file.gcount() != sizeof(vlen)) break;
            
            value.resize(vlen);
            file.read(&value[0], vlen);
            if (file.gcount() != vlen) break;
        } else {
            // Skip dummy vlen for delete
            uint32_t vlen;
            file.read(reinterpret_cast<char*>(&vlen), sizeof(vlen));
            if (file.gcount() != sizeof(vlen)) break;
        }
        
        // Apply to memtable
        if (type == 0) {
            _memtable->Put(key, value);
        } else {
            _memtable->Delete(key);
        }
        
        valid_pos = file.tellg();
    }
    
    file.close();
    
    // Truncate partial writes
    if (fs::file_size(wal_path) != valid_pos) {
        fs::resize_file(wal_path, valid_pos);
        std::cout << "[C++] Recovered WAL, truncated to " << valid_pos << " bytes" << std::endl;
    }
}

void DB::MaybeScheduleCompaction() {
    _compaction_cv.notify_all();
}

void DB::BackgroundCompaction() {
    while (!_stop_compaction) {
        std::unique_ptr<VersionSet::Compaction> c;
        std::vector<Iterator*> iterators;
        
        {
            std::unique_lock<std::mutex> cv_lock(_compaction_mutex);
            _compaction_cv.wait(cv_lock, [this]{ 
                return _stop_compaction || (_versions && _versions->current()); // Simple check
            });
            if (_stop_compaction) break;
        }
        
        // Try pick compaction
        {
            std::lock_guard<std::mutex> db_lock(_mutex);
            _versions->current()->Finalize();
            if (_versions->current()->_compaction_score >= 1.0) {
                c = _versions->PickCompaction();
                if (c) {
                    std::cout << "[Compaction] Picked Level " << c->level << " (" << c->inputs[0].size() 
                               << " files) to merge with " << c->inputs[1].size() << " files in next level." << std::endl;
                     
                    for (const auto& f : c->inputs[0]) {
                        auto t = _versions->current()->GetTable(f.number);
                        if (t) iterators.push_back(t->NewIterator());
                    }
                    for (const auto& f : c->inputs[1]) {
                        auto t = _versions->current()->GetTable(f.number);
                        if (t) iterators.push_back(t->NewIterator());
                    }
                }
            }
        }
        
        if (!c || iterators.empty()) {
            // Nothing to do, wait again (or sleep briefly if we woke up spuriously)
             std::this_thread::sleep_for(std::chrono::milliseconds(100));
            continue;
        }
        
        // Merge
        MergingIterator* merge_iter = new MergingIterator(iterators);
        merge_iter->SeekToFirst();
        
        int file_num = _versions->NewFileNumber();
        std::string fname = _path + "/" + std::to_string(file_num) + ".sst";
        TableBuilder builder(fname);
        
        std::string smallest, largest;
        bool first = true;
        
        while (merge_iter->Valid()) {
            std::string key = merge_iter->Key();
            std::string value = merge_iter->Value();
            bool is_deleted = merge_iter->IsDeleted();
            
            // Skip duplicates (implied: first one is from lower level = newer)
            merge_iter->Next();
            while (merge_iter->Valid() && merge_iter->Key() == key) {
                merge_iter->Next();
            }
            
            if (first) { smallest = key; first = false; }
            largest = key;
            
            builder.Add(key, value, is_deleted);
        }
        delete merge_iter;
        builder.Finish();
        
        if (builder.FileSize() > 0) {
            std::lock_guard<std::mutex> db_lock(_mutex);
            Version* new_ver = new Version(*_versions->current());
            
            for (const auto& f : c->inputs[0]) new_ver->RemoveFile(c->level, f.number);
            for (const auto& f : c->inputs[1]) new_ver->RemoveFile(c->level + 1, f.number);
            
            FileMetaData meta;
            meta.number = file_num;
            meta.file_size = builder.FileSize();
            meta.smallest = smallest;
            meta.largest = largest;
            
            new_ver->AddFile(c->level + 1, meta);
            new_ver->SortL0();
            
            _versions->LogAndApply(new_ver);
            
            std::cout << "[Compaction] Committed L" << c->level << "->L" << c->level+1 
                      << " " << builder.FileSize() << " bytes." << std::endl;
        } else {
             fs::remove(fname);
        }
        
        MaybeScheduleCompaction(); // Check if more needed
    }
}

} // namespace lsm
