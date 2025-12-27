#pragma once
#include "message.h"
#include <vector>
#include <cstring>
#include <arpa/inet.h> // For htonl, ntohl

namespace geecache {
namespace rpc {

class Codec {
public:
    // Encode: Message -> Bytes
    static void Encode(const Message& msg, std::vector<char>& out) {
        uint64_t seq = msg.header.seq; 
        uint16_t method_len = static_cast<uint16_t>(msg.header.method.size());
        uint16_t error_len = static_cast<uint16_t>(msg.header.error.size());
        uint32_t body_len = static_cast<uint32_t>(msg.body.size());

        uint32_t total_len = 8 + 2 + method_len + 2 + error_len + body_len;

        // Reserve space
        size_t old_size = out.size();
        out.resize(old_size + 4 + total_len);
        char* ptr = out.data() + old_size;

        // Write Total Len (Big Endian)
        uint32_t net_total_len = htonl(total_len);
        memcpy(ptr, &net_total_len, 4); ptr += 4;

        // Write Seq (Raw/Little Endian)
        memcpy(ptr, &seq, 8); ptr += 8;

        // Write Method (Raw/Little Endian)
        memcpy(ptr, &method_len, 2); ptr += 2;
        if (method_len > 0) {
            memcpy(ptr, msg.header.method.data(), method_len); ptr += method_len;
        }

        // Write Error (Raw/Little Endian)
        memcpy(ptr, &error_len, 2); ptr += 2;
        if (error_len > 0) {
            memcpy(ptr, msg.header.error.data(), error_len); ptr += error_len;
        }

        // Write Body
        if (body_len > 0) {
            memcpy(ptr, msg.body.data(), body_len);
        }
    }

    // Decode: Bytes -> Message
    // Returns: >0 (bytes consumed), 0 (not enough data), -1 (error)
    static int Decode(const char* buf, size_t size, Message& msg) {
        if (size < 4) return 0;

        uint32_t total_len;
        memcpy(&total_len, buf, 4);
        total_len = ntohl(total_len);

        if (size < 4 + total_len) return 0; // Wait for more data

        const char* ptr = buf + 4;

        // Read Seq
        memcpy(&msg.header.seq, ptr, 8); ptr += 8;

        // Read Method
        uint16_t method_len;
        memcpy(&method_len, ptr, 2); ptr += 2;
        msg.header.method.assign(ptr, method_len); ptr += method_len;

        // Read Error
        uint16_t error_len;
        memcpy(&error_len, ptr, 2); ptr += 2;
        msg.header.error.assign(ptr, error_len); ptr += error_len;

        // Read Body
        size_t header_bytes = 8 + 2 + method_len + 2 + error_len;
        // Safety check
        if (header_bytes > total_len) return -1;

        size_t body_len = total_len - header_bytes;
        msg.body.assign(ptr, body_len);

        return 4 + total_len;
    }
};

}
}
