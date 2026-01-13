#pragma once
#include <string>
#include <cstdint>
#include <cstring>
#include <algorithm>
#include <climits>

namespace lsm {

// Provides encoding/decoding for Internal Keys.
// Format: UserKey + Fixed suffix (8 bytes)
// Suffix: BigEndian( ~SequenceNumber )
// We invert the sequence number so that lexicographical sort order (Ascending)
// results in: KeyA|Seq100 < KeyA|Seq99
// This way, the first entry found by Seek(KeyA) will be the one with the largest SequenceNumber.

class CodingUtil {
public:
    static std::string AppendSeq(const std::string& user_key, uint64_t seq) {
        std::string res = user_key;
        uint64_t s = ~seq; // Invert for descending order
        uint64_t be = __builtin_bswap64(s);
        res.append(reinterpret_cast<char*>(&be), sizeof(be));
        return res;
    }

    static std::string ExtractUserKey(const std::string& internal_key) {
        if (internal_key.size() < 8) return internal_key;
        return internal_key.substr(0, internal_key.size() - 8);
    }
    
    static uint64_t ExtractSeq(const std::string& internal_key) {
        if (internal_key.size() < 8) return 0;
        uint64_t be;
        memcpy(&be, internal_key.data() + internal_key.size() - 8, 8);
        return ~(__builtin_bswap64(be));
    }
};

} // namespace lsm
