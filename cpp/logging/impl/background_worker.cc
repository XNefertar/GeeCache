#include "background_worker.h"
#include <cassert>
#include <chrono>
#include <cstdio>

namespace lsm {
    BackgroundWorker::BackgroundWorker(int flushInterval)
        : _running(false)
        , _flushInterval(flushInterval)
        , _currentBuffer(std::make_unique<FixedBuffer<kBufferSize>>())
        , _nextBuffer(std::make_unique<FixedBuffer<kBufferSize>>()) {
            _currentBuffer->reset();
            _nextBuffer->reset();
            _buffers.reserve(16);
    }

    BackgroundWorker::~BackgroundWorker(){
        if (_running.load()) {
            stop();
        }
    }

    void BackgroundWorker::start() {
        if (_thread.joinable()) {
            return;
        }
        _running.store(true);
        _thread = std::thread(&BackgroundWorker::threadLoop, this);
    }

    void BackgroundWorker::stop() {
        _running.store(false);
        _cond.notify_all();
        if (_thread.joinable()) {
            _thread.join();
        }
    }

    void BackgroundWorker::addSink(std::shared_ptr<LogSink> sink) {
        std::lock_guard<std::mutex> lock(_sinksMutex);
        _sinks.push_back(std::move(sink));
    }

    void BackgroundWorker::append(const char *data, int len) {
        std::lock_guard<std::mutex> lock(_mutex);

        if (_currentBuffer->avail() > len) {
            _currentBuffer->append(data, len);
        } else {
            _buffers.push_back(std::move(_currentBuffer));

            if (_nextBuffer) {
                _currentBuffer = std::move(_nextBuffer);
            } else {
                _currentBuffer = std::make_unique<FixedBuffer<kBufferSize>>();
            }

            _currentBuffer->append(data, len);
            _cond.notify_all();
        }
    }

    void BackgroundWorker::threadLoop() {
        BufferPtr newBuffer1 = std::make_unique<Buffer>();
        BufferPtr newBuffer2 = std::make_unique<Buffer>();
        newBuffer1->reset();
        newBuffer2->reset();

        BufferVector buffersToWrite;
        buffersToWrite.reserve(16);

        while (_running.load()) {
            {
                std::unique_lock<std::mutex> lock(_mutex);

                if (_buffers.empty()) {
                    _cond.wait_for(lock, std::chrono::seconds(_flushInterval));
                }

                _buffers.push_back(std::move(_currentBuffer));
                _currentBuffer = std::move(newBuffer1);
                buffersToWrite.swap(_buffers);

                if (!_nextBuffer) {
                    _nextBuffer = std::move(newBuffer2);
                }
            }

            if (buffersToWrite.size() > 25) {
                char buf[256];
                snprintf(buf, sizeof(buf), "Dropped %zd long buffers\n", buffersToWrite.size() - 2);
                fputs(buf, stderr);
                buffersToWrite.erase(buffersToWrite.begin() + 2, buffersToWrite.end());
            }

            // Copy sinks under lock to avoid holding lock during IO
            std::vector<std::shared_ptr<LogSink>> sinksCopy;
            {
                std::lock_guard<std::mutex> lock(_sinksMutex);
                sinksCopy = _sinks;
            }

            for (const auto &buffer : buffersToWrite) {
                for (auto &sink : sinksCopy) {
                    sink->Write(buffer->data(), buffer->length());
                }
            }

            if (buffersToWrite.size() > 2) {
                for (auto &sink : sinksCopy) {
                    sink->Flush();
                }
                buffersToWrite.resize(2);
            }

            if (!newBuffer1) {
                assert(!buffersToWrite.empty());
                newBuffer1 = std::move(buffersToWrite.back());
                buffersToWrite.pop_back();
                newBuffer1->reset();
            }

            if (!newBuffer2) {
                assert(!buffersToWrite.empty());
                newBuffer2 = std::move(buffersToWrite.back());
                buffersToWrite.pop_back();
                newBuffer2->reset();
            }

            buffersToWrite.clear();
        }

        // Final flush: copy sinks under lock then flush
        {
            std::lock_guard<std::mutex> lock(_sinksMutex);
            for (auto &sink : _sinks) {
                sink->Flush();
            }
        }
    }
}