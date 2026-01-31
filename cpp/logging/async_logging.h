#pragma once

#include <string>

namespace lsm {

    // Initializes the asynchronous logging system.
    // basename: The log file name (e.g. "geecache.log")
    // flushInterval: Seconds to wait before forcing a flush to disk.
    void setupAsyncLogging(const std::string& basename, int flushInterval = 3);

}
