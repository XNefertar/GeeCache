#include "db.h"
#include <iostream>
#include <filesystem>
#include <chrono>
#include <fstream>
#include <climits>
#include "core/sstable/table_builder.h"
#include "core/sstable/table.h"
#include "util/merging_iterator.h"
#include "core/coding.h"

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
    
    _wal = std::make_shared<WAL>(wal_path);

    if (_options.write_rate_limit > 0) {
        // Capacity should be at least allow small chunks.
        double burst_duration_seconds = 1.0;
        double capacity = static_cast<double>(_options.write_rate_limit) * burst_duration_seconds;
        _rate_limiter = std::make_unique<TokenBucket>(capacity, _options.write_rate_limit);
        // std::cout << "[C++] Write rate limit enabled: " << _options.write_rate_limit 
        //           << " B/s, Capacity: " << capacity << " B" << std::endl;
    }
    
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
    if (_rate_limiter) {
        // 使用 Request 替代原来的忙等/轮询逻辑
        // Request 内部计算需要等待的时间并进行精准睡眠，避免 CPU 浪费和延迟抖动
        _rate_limiter->Request(key.size() + value.size());
    }

    std::lock_guard<std::mutex> lock(_mutex);
    
    if (_memtable->MemoryUsage() >= kMemTableSizeLimit) {
        Flush();
    }

    uint64_t seq = ++_last_seq;
    std::string internal_key = CodingUtil::AppendSeq(key, seq);

    _wal->Append(internal_key, value, false);
    if (_options.sync) {
        _wal->Sync();
    }
    _memtable->Put(internal_key, value);
}

bool DB::Get(const std::string& key, std::string* value) {
    std::lock_guard<std::mutex> lock(_mutex);
    
    std::string lookup_key = CodingUtil::AppendSeq(key, UINT64_MAX);

    // Check MemTable
    {
        std::unique_ptr<SkipList::Iterator> iter(_memtable->NewIterator());
        iter->Seek(lookup_key);
        if (iter->Valid()) {
            std::string internal_key = iter->Key();
            if (CodingUtil::ExtractUserKey(internal_key) == key) {
                 if (iter->IsDeleted()) {
                     return false;
                 }
                 *value = iter->Value();
                 return true;
            }
        }
    }

    // Check SSTables via Version
    Table::Status result = _versions->current()->Get(lookup_key, value);
    if (result == Table::kFound) {
        return true;
    } else if (result == Table::kDeleted) {
        value->clear();
        return false;
    }
    return false;
}

void DB::Delete(const std::string& key) {
    std::lock_guard<std::mutex> lock(_mutex);
    
    if (_memtable->MemoryUsage() >= kMemTableSizeLimit) {
        Flush();
    }

    uint64_t seq = ++_last_seq;
    std::string internal_key = CodingUtil::AppendSeq(key, seq);

    _wal->Append(internal_key, "", true);
    if (_options.sync) {
        _wal->Sync();
    }
    _memtable->Delete(internal_key);
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
    _wal = std::make_shared<WAL>(wal_path);
    
    std::cout << "[C++] Flushed MemTable to " << fname << std::endl;
    
    MaybeScheduleCompaction();
}

void DB::BackgroundSync() {
    while (!_stop_sync) {
        std::this_thread::sleep_for(std::chrono::seconds(1));
        if (_stop_sync) break;

        std::shared_ptr<WAL> current_wal;
        {
            std::lock_guard<std::mutex> lock(_mutex);
            current_wal = _wal;
        }

        if (current_wal) {
            current_wal->Sync();
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
        
        // Recover Max Seq
        uint64_t seq = CodingUtil::ExtractSeq(key);
        if (seq > _last_seq) _last_seq = seq;
        
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
    {
        std::lock_guard<std::mutex> lock(_compaction_mutex);
        _compaction_scheduled = true;
    }
    _compaction_cv.notify_all();
}

void DB::BackgroundCompaction() {
    while (!_stop_compaction) {
        std::unique_ptr<VersionSet::Compaction> c;
        std::vector<std::unique_ptr<Iterator>> iterators;
        int file_num = 0;
        
        {
            std::unique_lock<std::mutex> cv_lock(_compaction_mutex);
            _compaction_cv.wait(cv_lock, [this]{ 
                return _stop_compaction || _compaction_scheduled;
            });
            if (_stop_compaction) break;

            _compaction_scheduled = false;
        }
        
        // Try pick compaction
        {
            std::lock_guard<std::mutex> db_lock(_mutex);
            _versions->current()->Finalize();
            if (_versions->current()->_compaction_score >= 1.0) {
                c = _versions->PickCompaction();
                if (c) {
                    file_num = _versions->NewFileNumber();
                    std::cout << "[Compaction] Picked Level " << c->level << " (" << c->inputs[0].size() 
                               << " files) to merge with " << c->inputs[1].size() << " files in next level." << std::endl;
                    
                    bool error = false;
                    auto add_iterators = [&](const std::vector<FileMetaData>& files) {
                        // Iterate in reverse order (newest to oldest) so that newer files
                        // get lower indices in MergingIterator (higher priority).
                        for (auto it = files.rbegin(); it != files.rend(); ++it) {
                            const auto& f = *it;
                            auto t = _versions->current()->GetTable(f.number);
                            if (!t) {
                                std::cerr << "[Compaction Error] Failed to open table " << f.number 
                                          << ". Aborting compaction." << std::endl;
                                error = true;
                                return;
                            }
                            iterators.push_back(std::unique_ptr<Iterator>(t->NewIterator()));
                        }
                    };

                    add_iterators(c->inputs[0]);
                    if (!error) {
                        add_iterators(c->inputs[1]);
                    }
                    
                    if (error) {
                        iterators.clear();
                        c.reset();
                    }
                }
            }
        }
        
        if (!c || iterators.empty()) {
            continue;
        }
        
        // Merge
        MergingIterator* merge_iter = new MergingIterator(std::move(iterators));
        merge_iter->SeekToFirst();
        
        std::string fname = _path + "/" + std::to_string(file_num) + ".sst";
        TableBuilder builder(fname);
        
        std::string smallest, largest;
        bool first = true;
        
        std::string current_user_key;
        bool has_current_user_key = false;
        
        while (merge_iter->Valid()) {
            std::string internal_key = merge_iter->Key();
            std::string user_key = CodingUtil::ExtractUserKey(internal_key);
            std::string value = merge_iter->Value();
            bool is_deleted = merge_iter->IsDeleted();
            
            // Note: Keys are sorted by UserKey ASC, Seq DESC.
            // The first time we see a User Key, it is the newest version.
            bool is_new_key = (!has_current_user_key || user_key != current_user_key);
            
            if (is_new_key) {
                current_user_key = user_key;
                has_current_user_key = true;
                
                // Drop tombstone if it is safe (bottom level)
                bool drop = false;
                if (is_deleted) {
                    // Check if we are compacting to the last level (Level 6)
                    // We assume kNumLevels = 7 (0..6)
                    if (c->level + 1 >= 6) {
                        drop = true;
                    }
                }
                
                if (!drop) {
                    if (first) { smallest = internal_key; first = false; }
                    largest = internal_key;
                    builder.Add(internal_key, value, is_deleted);
                }
            } else {
                // This is an older version of the same User Key (lower sequence number).
                // It is shadowed by the newer version we just processed.
                // Drop it.
            }
            
            merge_iter->Next();
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
            new_ver->SortLevel(c->level + 1);
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
