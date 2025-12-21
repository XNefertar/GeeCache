#pragma once
#include <string>
#include <map>
#include <mutex>

namespace lsm {

class DB {
public:
    DB(const std::string& path);
    ~DB();

    void Put(const std::string& key, const std::string& value);
    bool Get(const std::string& key, std::string* value);
    void Delete(const std::string& key);

private:
    std::string path_;
    std::map<std::string, std::string> data_;
    std::mutex mutex_;
};

} // namespace lsm
