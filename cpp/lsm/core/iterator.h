#pragma once
#include <string>

namespace lsm {

    class Iterator {
    public:
        virtual ~Iterator() {}
        virtual bool Valid() const = 0;
        virtual void SeekToFirst() = 0;
        virtual void Seek(const std::string& target) = 0;
        virtual void Next() = 0;
        virtual std::string Key() const = 0;
        virtual std::string Value() const = 0;
        // For tombstone handling
        virtual bool IsDeleted() const { return false; }
    };

}
