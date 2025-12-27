#pragma once
#include "socket.h"
#include "message.h"
#include "codec.h"
#include <map>
#include <functional>
#include <thread>
#include <mutex>
#include <iostream>

namespace geecache {
namespace rpc {

class Server {
public:
    using Handler = std::function<void(const std::string& args, std::string& reply)>;

    void Register(const std::string& method, Handler handler) {
        std::lock_guard<std::mutex> lock(mu_);
        handlers_[method] = handler;
    }

    void Run(int port) {
        Socket listener;
        listener.Bind(port);
        listener.Listen();
        std::cout << "RPC Server running on port " << port << std::endl;

        while (true) {
            try {
                Socket client = listener.Accept();
                // Move client socket to thread
                // We need to detach or join. Detach is easier for simple server.
                // But Socket destructor closes fd. We need to transfer ownership.
                // Socket has no copy constructor (implicit deleted because of int fd?), 
                // actually I didn't delete it but it should be unique.
                // My Socket class has a destructor that closes. Copying it would be bad (double close).
                // I should make it move-only.
                
                // Let's fix Socket class first or just use raw fd in thread.
                // I'll modify Socket to be move-only or use shared_ptr.
                // For now, I'll just extract fd.
                int fd = client.fd();
                // Prevent destructor from closing it by setting fd to -1 in the local object
                // But I don't have a release() method.
                // I'll just use std::thread with a lambda that takes ownership if I implement move ctor.
                
                // Let's assume I'll fix Socket to be movable.
                // For now, I'll just hack it:
                // The Socket object 'client' will be destroyed at end of scope.
                // I need to keep it alive or move it.
                
                std::thread t(&Server::HandleConnection, this, std::move(client));
                t.detach();
            } catch (const std::exception& e) {
                std::cerr << "Accept error: " << e.what() << std::endl;
            }
        }
    }

private:
    void HandleConnection(Socket client) {
        std::vector<char> buffer;
        std::vector<char> read_buf(4096);

        while (true) {
            ssize_t n = client.Recv(read_buf.data(), read_buf.size());
            if (n <= 0) break;

            buffer.insert(buffer.end(), read_buf.begin(), read_buf.begin() + n);

            while (true) {
                Message req;
                int consumed = Codec::Decode(buffer.data(), buffer.size(), req);
                if (consumed == 0) break; // Need more data
                if (consumed < 0) return; // Error
                
                // Process request
                Message resp;
                resp.header.seq = req.header.seq;
                resp.header.method = req.header.method;

                std::string reply_body;
                std::string error_msg;

                Handler handler;
                {
                    std::lock_guard<std::mutex> lock(mu_);
                    auto it = handlers_.find(req.header.method);
                    if (it != handlers_.end()) {
                        handler = it->second;
                    }
                }

                if (handler) {
                    try {
                        handler(req.body, reply_body);
                    } catch (const std::exception& e) {
                        error_msg = e.what();
                    }
                } else {
                    error_msg = "Method not found: " + req.header.method;
                }

                resp.header.error = error_msg;
                resp.body = reply_body;

                std::vector<char> out;
                Codec::Encode(resp, out);
                if (!client.SendAll(out.data(), out.size())) return;

                // Remove consumed bytes
                buffer.erase(buffer.begin(), buffer.begin() + consumed);
            }
        }
    }

    std::mutex mu_;
    std::map<std::string, Handler> handlers_;
};

}
}
