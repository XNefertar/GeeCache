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
    
private:
    static const int kMaxLevel = 12;
    Node* head_;
    int level_;
    std::mt19937 rng_;
    std::uniform_int_distribution<> dist_;

    int RandomLevel();
};

} // namespace lsm
