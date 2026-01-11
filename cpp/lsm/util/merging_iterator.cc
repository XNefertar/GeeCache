#include "util/merging_iterator.h"

namespace lsm {

MergingIterator::MergingIterator(const std::vector<Iterator*>& children) 
    : _children(children), _current(nullptr) {
}

MergingIterator::~MergingIterator() {
    for (auto child : _children) {
        delete child;
    }
}

bool MergingIterator::Valid() const {
    return _current != nullptr && _current->Valid();
}

void MergingIterator::SeekToFirst() {
    while (!_heap.empty()) _heap.pop();
    
    for (size_t i = 0; i < _children.size(); ++i) {
        _children[i]->SeekToFirst();
        if (_children[i]->Valid()) {
            _heap.push({_children[i], (int)i});
        }
    }
    
    if (!_heap.empty()) {
        _current = _heap.top().iter;
    } else {
        _current = nullptr;
    }
}

void MergingIterator::Seek(const std::string& target) {
    while (!_heap.empty()) _heap.pop();
    
    for (size_t i = 0; i < _children.size(); ++i) {
        _children[i]->Seek(target);
        if (_children[i]->Valid()) {
            _heap.push({_children[i], (int)i});
        }
    }
    
    if (!_heap.empty()) {
        _current = _heap.top().iter;
    } else {
        _current = nullptr;
    }
}

void MergingIterator::Next() {
    if (_heap.empty()) return;
    
    Node top = _heap.top();
    _heap.pop();
    
    // Advance the current iterator
    top.iter->Next();
    if (top.iter->Valid()) {
        _heap.push(top);
    }
    
    if (!_heap.empty()) {
        _current = _heap.top().iter;
    } else {
        _current = nullptr;
    }
}

std::string MergingIterator::Key() const {
    return _current->Key();
}

std::string MergingIterator::Value() const {
    return _current->Value();
}

bool MergingIterator::IsDeleted() const {
    return _current->IsDeleted();
}

}
