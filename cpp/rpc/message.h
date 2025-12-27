#pragma once
#include <string>
#include <cstdint>

namespace geecache {
namespace rpc {

struct Header {
    uint64_t seq = 0;
    std::string method;
    std::string error;
};

struct Message {
    Header header;
    std::string body;
};

}
}
