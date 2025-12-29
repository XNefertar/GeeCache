#pragma once
#include <string>
#include <map>
#include <cstdint>

namespace geecache {
namespace rpc {

// Protocol Constants
const uint16_t MAGIC = 0x4743; // "GC"
const uint8_t VERSION = 1;

// Flags
const uint8_t FLAG_RESPONSE = 0x01;
const uint8_t FLAG_ERROR = 0x02;

struct Message {
    uint64_t seq = 0;
    uint8_t flags = 0;
    
    // Dynamic metadata (Method, Auth, Tracing, etc.)
    std::map<std::string, std::string> meta;
    
    // Payload
    std::string body;

    bool IsResponse() const { return flags & FLAG_RESPONSE; }
    bool IsError() const { return flags & FLAG_ERROR; }
    void SetResponse() { flags |= FLAG_RESPONSE; }
    void SetError() { flags |= FLAG_ERROR; }
};

}
}
