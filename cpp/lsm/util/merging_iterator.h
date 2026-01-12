#pragma once
#include <vector>
#include <queue>
#include <memory>
#include <functional>
#include "core/iterator.h"

namespace lsm {

class MergingIterator : public Iterator {
public:
    // Takes ownership of children
    MergingIterator(std::vector<std::unique_ptr<Iterator>> children);
    virtual ~MergingIterator();

    bool Valid() const override;
    void SeekToFirst() override;
    void Seek(const std::string& target) override;
    void Next() override;
    std::string Key() const override;
    std::string Value() const override;
    bool IsDeleted() const override;

private:
    std::vector<std::unique_ptr<Iterator>> _children;
    
    struct Node {
        Iterator* iter;
        int index; // Lower index = newer data (higher priority)
        
        bool operator>(const Node& other) const {
            if (iter->Key() != other.iter->Key()) {
                return iter->Key() > other.iter->Key();
            }
            // Key is same, we want smaller index to be "smaller" (top of heap)
            // operator> for min-heap means "is this larger than other?"
            // We want smaller index at top, so "invalid/larger" condition is index > other.index
            return index > other.index;
        }
    };
    
    std::priority_queue<Node, std::vector<Node>, std::greater<Node>> _heap;
    Iterator* _current;
    
    // Direction is always forward for now
};

}
