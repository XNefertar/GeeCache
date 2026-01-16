#include "sinks/file_sink.h"
#include <cstdio>
#include <cassert>

namespace lsm {

FileSink::FileSink(const std::string& filename)
    : _file(fopen(filename.c_str(), "ae")) { // append + cloexec
    if (!_file) {
        // Fallback or throw?
        fprintf(stderr, "Failed to open log file: %s\n", filename.c_str());
    }
}

FileSink::~FileSink() {
    if (_file) {
        fclose(_file);
    }
}

void FileSink::Write(const char* data, size_t len) {
    if (_file) {
        // unlocked_fwrite is faster but not thread safe.
        // FileSink is usually written to by a single thread (AsyncLogging thread).
        // But if multiple threads write (Sync logging), we need lock.
        // Assuming AsyncLogging -> Single thread.
        fwrite(data, 1, len, _file);
    }
}

void FileSink::Flush() {
    if (_file) {
        fflush(_file);
    }
}

}