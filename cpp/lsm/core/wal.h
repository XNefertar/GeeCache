#pragma once
#include <string>
#include <mutex>

namespace lsm {

class WAL {
public:
    WAL(const std::string& path);
    ~WAL();

    void Append(const std::string& key, const std::string& value, bool is_delete);
    void Sync();
    
private:
    std::string _path;
    int _fd;
    std::mutex _mutex;
};

} // namespace lsm
