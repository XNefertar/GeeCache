#include "skiplist.h"
#include <ctime>

namespace lsm {

SkipList::SkipList() : level_(1), rng_(std::time(nullptr)), dist_(0, 1) {
    head_ = new Node("", "", false, kMaxLevel);
}

SkipList::~SkipList() {
    Node* current = head_;
    while (current) {
        Node* next = current->next[0];
        delete current;
        current = next;
    }
}

int SkipList::RandomLevel() {
    int lvl = 1;
    while (dist_(rng_) && lvl < kMaxLevel) {
        lvl++;
    }
    return lvl;
}

void SkipList::Insert(const std::string& key, const std::string& value, bool is_deleted) {
    std::vector<Node*> update(kMaxLevel);
    Node* current = head_;

    for (int i = level_ - 1; i >= 0; i--) {
        while (current->next[i] && current->next[i]->key < key) {
            current = current->next[i];
        }
        update[i] = current;
    }

    current = current->next[0];

    if (current && current->key == key) {
        current->value = value;
        current->is_deleted = is_deleted;
    } else {
        int new_level = RandomLevel();
        if (new_level > level_) {
            for (int i = level_; i < new_level; i++) {
                update[i] = head_;
            }
            level_ = new_level;
        }

        Node* new_node = new Node(key, value, is_deleted, new_level);
        for (int i = 0; i < new_level; i++) {
            new_node->next[i] = update[i]->next[i];
            update[i]->next[i] = new_node;
        }
    }
}

bool SkipList::Get(const std::string& key, std::string* value) {
    Node* current = head_;
    for (int i = level_ - 1; i >= 0; i--) {
        while (current->next[i] && current->next[i]->key < key) {
            current = current->next[i];
        }
    }
    current = current->next[0];

    if (current && current->key == key) {
        if (current->is_deleted) {
            return false;
        }
        *value = current->value;
        return true;
    }
    return false;
}

} // namespace lsm
