#pragma once
#include "message.h"
#include <vector>
#include <cstring>
#include <arpa/inet.h> // For htonl, ntohl

namespace geecache {
namespace rpc {

class Codec {
public:
    // Frame Format (v2):
    // [Magic 2] [Ver 1] [Flags 1] [TotalLen 4]
    // [Seq 8]
    // [MetaLen 2]
    // [MetaBytes (KeyLen2+Key+ValLen2+Val)...]
    // [BodyBytes...]

    static void Encode(const Message& msg, std::vector<char>& out) {
        // 1. Serialize Metadata
        std::vector<char> meta_buf;
        for (const auto& pair : msg.meta) {
            uint16_t klen = pair.first.size();
            uint16_t vlen = pair.second.size();
            
            size_t old = meta_buf.size();
            meta_buf.resize(old + 2 + klen + 2 + vlen);
            char* p = meta_buf.data() + old;
            
            memcpy(p, &klen, 2); p += 2;
            memcpy(p, pair.first.data(), klen); p += klen;
            memcpy(p, &vlen, 2); p += 2;
            memcpy(p, pair.second.data(), vlen);
        }

        // 2. Calculate Lengths
        uint32_t meta_len = meta_buf.size();
        uint32_t body_len = msg.body.size();
        // TotalLen = Seq(8) + MetaLen(2) + MetaBytes + BodyBytes
        uint32_t total_len = 8 + 2 + meta_len + body_len;

        // 3. Allocate Output
        size_t old_size = out.size();
        out.resize(old_size + 8 + total_len); // 8 is Fixed Header size
        char* ptr = out.data() + old_size;

        // 4. Write Fixed Header
        uint16_t magic = htons(MAGIC);
        memcpy(ptr, &magic, 2); ptr += 2;
        
        *ptr++ = VERSION;
        *ptr++ = msg.flags;
        
        uint32_t net_total_len = htonl(total_len);
        memcpy(ptr, &net_total_len, 4); ptr += 4;

        // 5. Write Seq
        memcpy(ptr, &msg.seq, 8); ptr += 8;

        // 6. Write Meta
        uint16_t net_meta_len = meta_len; // Use native endian for internal fields for simplicity, or standard?
        // Let's stick to raw copy for internal fields as per previous design, 
        // BUT for a "mature" protocol, we should handle endianness.
        // For simplicity in this demo, I'll assume homogeneous arch (x86_64).
        memcpy(ptr, &net_meta_len, 2); ptr += 2;
        if (meta_len > 0) {
            memcpy(ptr, meta_buf.data(), meta_len); ptr += meta_len;
        }

        // 7. Write Body
        if (body_len > 0) {
            memcpy(ptr, msg.body.data(), body_len);
        }
    }

    static int Decode(const char* buf, size_t size, Message& msg) {
        if (size < 8) return 0;

        const char* ptr = buf;

        // 1. Check Magic
        uint16_t magic;
        memcpy(&magic, ptr, 2); ptr += 2;
        if (ntohs(magic) != MAGIC) return -1; // Bad Magic

        // 2. Check Version
        uint8_t ver = *ptr++;
        if (ver != VERSION) return -1; // Unsupported Version

        // 3. Read Flags
        msg.flags = *ptr++;

        // 4. Read Total Length
        uint32_t total_len;
        memcpy(&total_len, ptr, 4); ptr += 4;
        total_len = ntohl(total_len);

        if (size < 8 + total_len) return 0; // Wait for more data

        // 5. Read Seq
        memcpy(&msg.seq, ptr, 8); ptr += 8;

        // 6. Read Meta
        uint16_t meta_len;
        memcpy(&meta_len, ptr, 2); ptr += 2;
        
        const char* meta_end = ptr + meta_len;
        while (ptr < meta_end) {
            uint16_t klen;
            memcpy(&klen, ptr, 2); ptr += 2;
            std::string key(ptr, klen); ptr += klen;
            
            uint16_t vlen;
            memcpy(&vlen, ptr, 2); ptr += 2;
            std::string val(ptr, vlen); ptr += vlen;
            
            msg.meta[key] = val;
        }

        // 7. Read Body
        size_t body_len = total_len - 8 - 2 - meta_len;
        msg.body.assign(ptr, body_len);

        return 8 + total_len;
    }
};

}
}
