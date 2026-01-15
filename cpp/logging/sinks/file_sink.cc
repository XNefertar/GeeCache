#include "logging.h"
#include <cstdio>

namespace lsm {
    class FileSink : public LogSink {
    public:
        void Write(const char* data, size_t len) override {
            // fwrite(data, 1, len, _file);
        }
    };
}