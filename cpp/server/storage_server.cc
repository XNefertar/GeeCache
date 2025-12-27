#include "../lsm/include/lsm.h"
#include "../rpc/socket.h"
#include "../rpc/codec.h"
#include <iostream>
#include <thread>
#include <vector>
#include <string>
#include <cstring>
#include <stdexcept>

using namespace geecache::rpc;

// RAII Wrapper for LSM C API
class LsmEngine {
public:
    LsmEngine(const std::string& path) {
        lsm_options_t* opts = lsm_options_create();
        lsm_options_set_create_if_missing(opts, 1);
        char* err = nullptr;
        db_ = lsm_db_open(opts, path.c_str(), &err);
        lsm_options_destroy(opts); // Options are copied or no longer needed
        
        if (err) {
            std::string msg = err;
            lsm_free(err);
            throw std::runtime_error("DB Open failed: " + msg);
        }
    }

    ~LsmEngine() {
        if (db_) {
            lsm_db_close(db_);
        }
    }

    // Disable copy
    LsmEngine(const LsmEngine&) = delete;
    LsmEngine& operator=(const LsmEngine&) = delete;

    std::string Get(const std::string& key) {
        size_t vallen = 0;
        char* err = nullptr;
        char* val = lsm_get(db_, key.data(), key.size(), &vallen, &err);
        
        if (err) {
            std::string msg = err;
            lsm_free(err);
            throw std::runtime_error(msg);
        }
        
        if (!val) {
            throw std::runtime_error("Key not found");
        }

        std::string result(val, vallen);
        lsm_free(val);
        return result;
    }

    void Put(const std::string& key, const std::string& val) {
        char* err = nullptr;
        lsm_put(db_, key.data(), key.size(), val.data(), val.size(), &err);
        
        if (err) {
            std::string msg = err;
            lsm_free(err);
            throw std::runtime_error(msg);
        }
    }

    void Delete(const std::string& key) {
        char* err = nullptr;
        lsm_delete(db_, key.data(), key.size(), &err);
        
        if (err) {
            std::string msg = err;
            lsm_free(err);
            throw std::runtime_error(msg);
        }
    }

private:
    lsm_db_t* db_ = nullptr;
};

class StorageServer {
public:
    StorageServer(int port, const std::string& db_path) 
        : port_(port), engine_(db_path) {}

    void Run() {
        Socket listener;
        listener.Bind(port_);
        listener.Listen();
        std::cout << "Storage Server listening on " << port_ << "..." << std::endl;

        while (true) {
            try {
                Socket client = listener.Accept();
                std::thread(&StorageServer::HandleClientWrapper, this, std::move(client)).detach();
            } catch (const std::exception& e) {
                std::cerr << "Accept error: " << e.what() << std::endl;
            }
        }
    }

private:
    void HandleClientWrapper(Socket sock) {
        HandleClient(sock);
    }

    void HandleClient(Socket& sock) {
        std::cout << "Client connected" << std::endl;
        std::vector<char> buffer;
        std::vector<char> read_buf(4096);

        while (true) {
            ssize_t n = sock.Recv(read_buf.data(), read_buf.size());
            if (n <= 0) {
                std::cout << "Client disconnected" << std::endl;
                break;
            }
            buffer.insert(buffer.end(), read_buf.begin(), read_buf.begin() + n);

            while (true) {
                Message req;
                int consumed = Codec::Decode(buffer.data(), buffer.size(), req);
                if (consumed <= 0) break;
                buffer.erase(buffer.begin(), buffer.begin() + consumed);

                Message resp;
                resp.header.seq = req.header.seq;
                
                try {
                    Process(req, resp);
                } catch (const std::exception& e) {
                    resp.header.error = e.what();
                }

                std::vector<char> out;
                Codec::Encode(resp, out);
                if (!sock.SendAll(out.data(), out.size())) return;
            }
        }
    }

    void Process(const Message& req, Message& resp) {
        if (req.header.method == "get") {
            resp.body = engine_.Get(req.body);
        } else if (req.header.method == "put") {
            if (req.body.size() < 4) throw std::runtime_error("Invalid body");
            uint32_t klen;
            memcpy(&klen, req.body.data(), 4);
            if (req.body.size() < 4 + klen) throw std::runtime_error("Invalid body length");
            
            std::string key = req.body.substr(4, klen);
            std::string val = req.body.substr(4 + klen);
            engine_.Put(key, val);
        } else if (req.header.method == "delete") {
            engine_.Delete(req.body);
        } else {
            throw std::runtime_error("Unknown method");
        }
    }

    int port_;
    LsmEngine engine_;
};

int main(int argc, char** argv) {
    int port = 9000;
    std::string db_path = "/tmp/geecache_lsm_data";
    
    if (argc > 1) port = std::stoi(argv[1]);
    if (argc > 2) db_path = argv[2];

    try {
        StorageServer server(port, db_path);
        server.Run();
    } catch (const std::exception& e) {
        std::cerr << "Fatal error: " << e.what() << std::endl;
        return 1;
    }
}
