#pragma once

#include "../logging.h"
#include <cstdio>
#include <string>

namespace lsm {

class FileSink : public LogSink {
 public:
  explicit FileSink(const std::string& filename);
  ~FileSink() override;

  void Write(const char* data, size_t len) override;
  void Flush() override;

 private:
  FILE* _file;
};

}
