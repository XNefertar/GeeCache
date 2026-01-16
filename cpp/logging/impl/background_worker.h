#pragma once

#include <vector>
#include <mutex>
#include <memory>
#include <thread>
#include <atomic>
#include <condition_variable>
#include "logging.h"
#include "ring_buffer.h"

namespace lsm {
    const int kBufferSize = 4 * 1024 * 1024; // 4MB

    class BackgroundWorker {
    public:
        BackgroundWorker(int flushInterval = 3);
        ~BackgroundWorker();

        void start();
        void stop();

        void addSink(std::shared_ptr<LogSink> sink);
        void append(const char *data, int len);

    private:
        void threadLoop();

        using Buffer = FixedBuffer<kBufferSize>;
        using BufferPtr = std::unique_ptr<Buffer>;
        using BufferVector = std::vector<BufferPtr>;

        std::mutex _mutex;
        std::condition_variable _cond;
        std::thread _thread;
        std::atomic<bool> _running;
        const int _flushInterval;

        BufferPtr _currentBuffer;
        BufferPtr _nextBuffer;
        BufferVector _buffers;

        std::vector<std::shared_ptr<LogSink>> _sinks;
        // protect _sinks from concurrent add/iterate
        std::mutex _sinksMutex;
    };
}