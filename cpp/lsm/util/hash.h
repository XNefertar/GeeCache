#pragma once
#include <cstdint>
#include <cstddef>

namespace lsm {

inline uint32_t Hash(const char* data, size_t n, uint32_t seed) {
    // Similar to MurmurHash3_x86_32
    const uint32_t m = 0x5bd1e995;
    const int r = 24;
    uint32_t h = seed ^ n;

    while (n >= 4) {
        uint32_t k = *(uint32_t*)data;
        k *= m;
        k ^= k >> r;
        k *= m;

        h *= m;
        h ^= k;

        data += 4;
        n -= 4;
    }

    switch (n) {
        case 3: h ^= data[2] << 16;
        case 2: h ^= data[1] << 8;
        case 1: h ^= data[0];
                h *= m;
    };

    h ^= h >> 13;
    h *= m;
    h ^= h >> 15;

    return h;
}

} // namespace lsm
