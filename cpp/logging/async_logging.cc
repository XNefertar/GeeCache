#include "async_logging.h"
#include "logging.h"
#include "impl/background_worker.h"
#include "sinks/file_sink.h"
#include <memory>
#include <atomic>
#include <cstring>
#include <mutex>

namespace lsm {

    static std::atomic<BackgroundWorker*> g_worker_ptr{nullptr};

    struct WorkerHolder {
        std::unique_ptr<BackgroundWorker> worker;
        ~WorkerHolder() {
            g_worker_ptr.store(nullptr, std::memory_order_release);
        }
    };

    static WorkerHolder g_worker_storage;
    static std::once_flag g_worker_once;

    const int kThreadBufferSize = 4096;

    struct ThreadBuffer {
        char buffer[kThreadBufferSize];
        int offset = 0;

        ~ThreadBuffer() {
            flush();
        }

        void flush() {
            if (offset > 0) {
                BackgroundWorker* worker = g_worker_ptr.load(std::memory_order_acquire);
                if (worker) {
                    worker->append(buffer, offset);
                }
                offset = 0;
            }
        }

        void append(const char* msg, int len) {
            BackgroundWorker* worker = g_worker_ptr.load(std::memory_order_acquire);
            if (!worker) {
                 fwrite(msg, 1, len, stdout);
                 return;
            }

            if (len >= kThreadBufferSize) {
                flush();
                worker->append(msg, len);
                return;
            }

            if (offset + len > kThreadBufferSize) {
                flush();
            }

            memcpy(buffer + offset, msg, len);
            offset += len;
        }
    };

    thread_local ThreadBuffer t_buffer;

    static void asyncOutput(const char* msg, size_t len) {
        t_buffer.append(msg, len);
    }


    void setupAsyncLogging(const std::string& basename, int flushInterval) {
        std::call_once(g_worker_once, [&]() {
            auto worker = std::make_unique<BackgroundWorker>(flushInterval);
            worker->addSink(std::make_shared<FileSink>(basename));
            worker->start();

            g_worker_ptr.store(worker.get(), std::memory_order_release);
            g_worker_storage.worker = std::move(worker);

            Logger::setOutput(asyncOutput);
        });
    }

}
