#include "util/merging_iterator.h"
#include <cassert>

namespace lsm {

MergingIterator::MergingIterator(std::vector<std::unique_ptr<Iterator>> children) 
    : _children(std::move(children)), _current(nullptr) {
}

MergingIterator::~MergingIterator() {
    // Unique pointers clean themselves up
}

bool MergingIterator::Valid() const {
    return _current != nullptr && _current->Valid();
}

void MergingIterator::SeekToFirst() {
    while (!_heap.empty()) _heap.pop();
    
    for (size_t i = 0; i < _children.size(); ++i) {
        _children[i]->SeekToFirst();
        if (_children[i]->Valid()) {
            _heap.push({_children[i].get(), (int)i});
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
            _heap.push({_children[i].get(), (int)i});
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
    
    // Remember the key we are currently ensuring we skip past
    std::string current_key = _current->Key();
    
    do {
        Node top = _heap.top();
        _heap.pop();
        
        // Advance the iterator
        top.iter->Next();
        if (top.iter->Valid()) {
            _heap.push(top);
        }
        
        // Update _current to point to the new top (or null)
        if (!_heap.empty()) {
            _current = _heap.top().iter;
        } else {
            _current = nullptr;
            return;
        }
        
        // If the new top has the same key (strictly equal, including sequence number if internal keys),
        // we must advance it too.
        // For standard merging iterator behavior, we treat strict equality as a duplicate to skip.
        // Note: In our InternalKey scheme (Key|~Seq), strictly equal keys means
        // same UserKey AND same Seq. This happens if we have overlapping L0 files
        // containing the exact same record (e.g. from forced flushes or simple duplication).
        
    } while (_current && _current->Key() == current_key);
}

std::string MergingIterator::Key() const {
    assert(_current != nullptr);
    return _current->Key();
}

std::string MergingIterator::Value() const {
    assert(_current != nullptr);
    return _current->Value();
}

bool MergingIterator::IsDeleted() const {
    assert(_current != nullptr);
    return _current->IsDeleted();
}

}
