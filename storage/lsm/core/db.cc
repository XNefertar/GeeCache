#include "db.h"
#include <iostream>
#include <filesystem>
#include <chrono>
#include <fstream>
#include "core/sstable/table_builder.h"

namespace lsm {

namespace fs = std::filesystem;

DB::DB(const std::string& path, const Options& options) 
    : _path(path), _options(options), _stop_sync(false) {
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
    
    std::cout << "[C++] DB opened at " << _path << std::endl;
}

DB::~DB() {
    _stop_sync = true;
    if (_sync_thread.joinable()) {
        _sync_thread.join();
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
    int result = _versions->current()->Get(key, value);
    if (result == 1) return true; // Found
    if (result == 2) return false; // Deleted
    return false; // Not found
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

} // namespace lsm
