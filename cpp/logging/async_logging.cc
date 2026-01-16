#include "async_logging.h"
#include "logging.h"
#include "impl/background_worker.h"
#include "sinks/file_sink.h"
#include <memory>

namespace lsm {

static std::unique_ptr<BackgroundWorker> g_worker;

static void asyncOutput(const char* msg, int len) {
    if (g_worker) {
        g_worker->append(msg, len);
    } else {
        // Fallback or lost
        fwrite(msg, 1, len, stdout);
    }
}

void setupAsyncLogging(const std::string& basename, int flushInterval) {
    if (g_worker) {
        return; // Already initialized
    }
    
    g_worker = std::make_unique<BackgroundWorker>(flushInterval);
    g_worker->addSink(std::make_shared<FileSink>(basename));
    
    Logger::setOutput(asyncOutput);
    
    g_worker->start();
}

}
