#pragma once

namespace lsm {
    class NullStream {
    public:
        template <typename T>
        NullStream operator<<(const T&) {
            return *this;
        }
    };
}