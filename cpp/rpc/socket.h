#pragma once
#include <string>
#include <vector>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <unistd.h>
#include <cstring>
#include <stdexcept>
#include <iostream>

namespace geecache {
namespace rpc {

class Socket {
public:
    Socket() : fd_(-1) {}
    explicit Socket(int fd) : fd_(fd) {}
    ~Socket() { Close(); }

    // Move constructor
    Socket(Socket&& other) noexcept : fd_(other.fd_) {
        other.fd_ = -1;
    }

    // Move assignment
    Socket& operator=(Socket&& other) noexcept {
        if (this != &other) {
            Close();
            fd_ = other.fd_;
            other.fd_ = -1;
        }
        return *this;
    }

    // Delete copy
    Socket(const Socket&) = delete;
    Socket& operator=(const Socket&) = delete;

    void Close() {
        if (fd_ != -1) {
            close(fd_);
            fd_ = -1;
        }
    }

    int fd() const { return fd_; }

    void Bind(int port) {
        fd_ = socket(AF_INET, SOCK_STREAM, 0);
        if (fd_ < 0) throw std::runtime_error("Failed to create socket");

        int opt = 1;
        setsockopt(fd_, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));

        sockaddr_in addr{};
        addr.sin_family = AF_INET;
        addr.sin_addr.s_addr = INADDR_ANY;
        addr.sin_port = htons(port);

        if (bind(fd_, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
            throw std::runtime_error("Failed to bind");
        }
    }

    void Listen() {
        if (listen(fd_, 10) < 0) {
            throw std::runtime_error("Failed to listen");
        }
    }

    Socket Accept() {
        sockaddr_in client_addr{};
        socklen_t len = sizeof(client_addr);
        int client_fd = accept(fd_, (struct sockaddr*)&client_addr, &len);
        if (client_fd < 0) {
            throw std::runtime_error("Failed to accept");
        }
        return Socket(client_fd);
    }

    void Connect(const std::string& ip, int port) {
        fd_ = socket(AF_INET, SOCK_STREAM, 0);
        if (fd_ < 0) throw std::runtime_error("Failed to create socket");

        sockaddr_in addr{};
        addr.sin_family = AF_INET;
        addr.sin_port = htons(port);
        inet_pton(AF_INET, ip.c_str(), &addr.sin_addr);

        if (connect(fd_, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
            throw std::runtime_error("Failed to connect");
        }
    }

    // Returns true if full buffer sent, false on error
    bool SendAll(const void* data, size_t len) {
        const char* ptr = static_cast<const char*>(data);
        size_t remaining = len;
        while (remaining > 0) {
            ssize_t sent = send(fd_, ptr, remaining, 0);
            if (sent <= 0) return false;
            ptr += sent;
            remaining -= sent;
        }
        return true;
    }

    // Returns bytes read, 0 on EOF, -1 on error
    ssize_t Recv(void* buf, size_t len) {
        return recv(fd_, buf, len, 0);
    }

private:
    int fd_;
};

}
}
