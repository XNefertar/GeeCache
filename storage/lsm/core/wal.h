#pragma once
#include <string>
#include <fstream>
#include <mutex>

namespace lsm {

class WAL {
public:
    WAL(const std::string& path);
    ~WAL();

    void Append(const std::string& key, const std::string& value, bool is_delete);
    
private:
    std::string path_;
    std::ofstream file_;
    std::mutex mutex_;
};

} // namespace lsm
