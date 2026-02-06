
#pragma once

#include "log_stream.h"
#include <string>

namespace lsm
{

    class LogSink
    {
    public:
        virtual void Write(const char *data, size_t len) = 0;
        virtual void Flush() = 0;
        virtual ~LogSink() = default;
    };

    enum class LogLevel
    {
        TRACE,
        DEBUG,
        INFO,
        WARN,
        ERROR,
        FATAL,
        NUM_LOG_LEVELS,
    };

    class Logger
    {
    public:
        Logger(const char *file, int line, LogLevel level);
        Logger(const char *file, int line, bool toAbort);
        ~Logger();

        LogStream &stream() { return _impl._stream; }

        static LogLevel logLevel();
        static void setLogLevel(LogLevel level);

        typedef void (*OutputFunc)(const char *msg, int len);
        typedef void (*FlushFunc)();
        static void setOutput(OutputFunc);
        static void setFlush(FlushFunc);

    private:
        class Impl {
        public:
            Impl(LogLevel level, int old_errno, const char *file, int line);
            void finish();

            LogStream _stream;
            LogLevel _level;
            int _line;
            std::string _basename;
        };

        Impl _impl;
    };

    void ConsoleOutput(const char *msg, int len);

    #define LOG_INFO                                       \
        if (lsm::Logger::logLevel() > lsm::LogLevel::INFO) {}        \
        else lsm::Logger(__FILE__, __LINE__, lsm::LogLevel::INFO).stream()

    #define LOG_DEBUG                                      \
        if (lsm::Logger::logLevel() > lsm::LogLevel::DEBUG) {}       \
        else lsm::Logger(__FILE__, __LINE__, lsm::LogLevel::DEBUG).stream()

    #define LOG_TRACE                                      \
        if (lsm::Logger::logLevel() > lsm::LogLevel::TRACE) {}       \
        else lsm::Logger(__FILE__, __LINE__, lsm::LogLevel::TRACE).stream()

    #define LOG_WARN lsm::Logger(__FILE__, __LINE__, lsm::LogLevel::WARN).stream()
    #define LOG_ERROR lsm::Logger(__FILE__, __LINE__, lsm::LogLevel::ERROR).stream()
    #define LOG_FATAL lsm::Logger(__FILE__, __LINE__, lsm::LogLevel::FATAL).stream()

} // namespace lsm