#pragma once
#include <string>
#include <vector>
#include <random>
#include <memory>

namespace lsm {

struct Node {
    std::string key;
    std::string value;
    bool is_deleted;
    std::vector<Node*> next;

    Node(const std::string& k, const std::string& v, bool del, int level)
        : key(k), value(v), is_deleted(del), next(level, nullptr) {}
};

class SkipList {
public:
    SkipList();
    ~SkipList();

    void Insert(const std::string& key, const std::string& value, bool is_deleted = false);
    bool Get(const std::string& key, std::string* value);

    class Iterator {
    public:
        explicit Iterator(const SkipList* list);
        bool Valid() const;
        void SeekToFirst();
        void Seek(const std::string& target);
        void Next();
        const std::string& Key() const;
        const std::string& Value() const;
        bool IsDeleted() const;
    private:
        const SkipList* _list;
        Node* _current;
    };

    Iterator* NewIterator() const;
    
    size_t MemoryUsage() const { return _memory_usage; }

private:
    static const int kMaxLevel = 12;
    Node* _head;
    int _level;
    size_t _memory_usage = 0;
    std::mt19937 _rng;
    std::uniform_int_distribution<> _dist;

    int RandomLevel();
};

} // namespace lsm
