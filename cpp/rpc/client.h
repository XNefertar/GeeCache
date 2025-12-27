#pragma once
#include "socket.h"
#include "message.h"
#include "codec.h"
#include <mutex>
#include <atomic>
#include <stdexcept>

namespace geecache {
namespace rpc {

class Client {
public:
    Client(const std::string& host, int port) {
        sock_.Connect(host, port);
    }

    void Call(const std::string& method, const std::string& args, std::string& reply) {
        std::lock_guard<std::mutex> lock(mu_);
        
        uint64_t seq = seq_++;
        Message req;
        req.header.seq = seq;
        req.header.method = method;
        req.body = args;

        std::vector<char> out;
        Codec::Encode(req, out);
        if (!sock_.SendAll(out.data(), out.size())) {
            throw std::runtime_error("Send failed");
        }

        // Wait for response
        // We might have buffered data from previous reads if we were async, 
        // but here we are synchronous and one-at-a-time.
        // However, we still need a buffer because a read might return partial data 
        // or more than one message (though unlikely if we wait for reply before sending next).
        
        while (true) {
            // Check if we have a complete message in buffer
            Message resp;
            int consumed = Codec::Decode(buffer_.data(), buffer_.size(), resp);
            
            if (consumed > 0) {
                // Remove consumed bytes
                buffer_.erase(buffer_.begin(), buffer_.begin() + consumed);
                
                if (resp.header.seq == seq) {
                    if (!resp.header.error.empty()) {
                        throw std::runtime_error(resp.header.error);
                    }
                    reply = resp.body;
                    return;
                } else {
                    // Unexpected sequence number. 
                    // In a strictly synchronous client, this shouldn't happen unless server is buggy 
                    // or we have leftover data.
                    // We'll just ignore it and keep reading? Or throw?
                    // Throwing is safer for now.
                    throw std::runtime_error("Seq mismatch");
                }
            } else if (consumed < 0) {
                throw std::runtime_error("Decode error");
            }

            // Read more
            std::vector<char> read_buf(4096);
            ssize_t n = sock_.Recv(read_buf.data(), read_buf.size());
            if (n <= 0) {
                throw std::runtime_error("Connection closed");
            }
            buffer_.insert(buffer_.end(), read_buf.begin(), read_buf.begin() + n);
        }
    }

private:
    Socket sock_;
    std::mutex mu_;
    std::atomic<uint64_t> seq_{0};
    std::vector<char> buffer_;
};

}
}
