#include "skiplist.h"
#include <ctime>

namespace lsm {

SkipList::SkipList() : _level(1), _rng(std::time(nullptr)), _dist(0, 1) {
    _head = new Node("", "", false, kMaxLevel);
}

SkipList::~SkipList() {
    Node* current = _head;
    while (current) {
        Node* next = current->next[0];
        delete current;
        current = next;
    }
}

int SkipList::RandomLevel() {
    int lvl = 1;
    while (_dist(_rng) && lvl < kMaxLevel) {
        lvl++;
    }
    return lvl;
}

void SkipList::Insert(const std::string& key, const std::string& value, bool is_deleted) {
    std::vector<Node*> update(kMaxLevel);
    Node* current = _head;

    for (int i = _level - 1; i >= 0; i--) {
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
        // 抛硬币决定新节点有多高
        int new_level = RandomLevel();

        // 只有当新高度比当前跳表总高度还高时，才需要处理
        if (new_level > _level) {
            // 补齐 update 数组
            for (int i = _level; i < new_level; i++) {
                update[i] = _head;
            }
            // 更新全局高度
            _level = new_level;
        }

        Node* new_node = new Node(key, value, is_deleted, new_level);
        // 循环每一层，把新节点"缝"进去
        for (int i = 0; i < new_level; i++) {
            // 新节点以此为继：右手拉住原来前驱的下家
            new_node->next[i] = update[i]->next[i];
            // 前驱以此为新：原来前驱的右手松开，改成拉住新节点
            update[i]->next[i] = new_node;
        }
    }
}

bool SkipList::Get(const std::string& key, std::string* value) {
    Node* current = _head;
    for (int i = _level - 1; i >= 0; i--) {
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

SkipList::Iterator::Iterator(const SkipList* list) : _list(list), _current(nullptr) {}

bool SkipList::Iterator::Valid() const {
    return _current != nullptr;
}

void SkipList::Iterator::SeekToFirst() {
    _current = _list->_head->next[0];
}

void SkipList::Iterator::Seek(const std::string& target) {
    Node* x = _list->_head;
    for (int i = _list->_level - 1; i >= 0; i--) {
        while (x->next[i] && x->next[i]->key < target) {
            x = x->next[i];
        }
    }
    _current = x->next[0];
}

void SkipList::Iterator::Next() {
    if (_current) {
        _current = _current->next[0];
    }
}

const std::string& SkipList::Iterator::Key() const {
    return _current->key;
}

const std::string& SkipList::Iterator::Value() const {
    return _current->value;
}

bool SkipList::Iterator::IsDeleted() const {
    return _current->is_deleted;
}

SkipList::Iterator* SkipList::NewIterator() const {
    return new Iterator(this);
}

} // namespace lsm
