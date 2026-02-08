#include "logging.h"

#include <cerrno>
#include <cstdio>
#include <cstring>
#include <ctime>
#include <sys/time.h>
#include <thread>
#include <atomic>
#include <cassert>

namespace lsm
{

    __thread char t_time[64];
    __thread time_t t_last_second;

    void ConsoleOutput(const char *msg, size_t len)
    {
        size_t n = fwrite(msg, 1, len, stdout);
        (void)n;
    }
    
    void defaultOutput(const char *msg, size_t len)
    {
        ConsoleOutput(msg, len);
    }

    void defaultFlush()
    {
        fflush(stdout);
    }

    std::atomic<Logger::OutputFunc> g_output{defaultOutput};
    std::atomic<Logger::FlushFunc> g_flush{defaultFlush};
    std::atomic<LogLevel> g_logLevel{LogLevel::INFO};

    const char *LogLevelName[static_cast<unsigned long>(LogLevel::NUM_LOG_LEVELS)] =
        {
            "TRACE ",
            "DEBUG ",
            "INFO  ",
            "WARN  ",
            "ERROR ",
            "FATAL ",
    };

    class T {
    public:
        T(const char *str, unsigned len)
            : str_(str),
              len_(len) {
            assert(strlen(str) == len_);
        }

        const char *str_;
        const unsigned len_;
    };

    LogStream &LogStream::operator<<(const T &t) {
        _buffer.append(t.str_, t.len_);
        return *this;
    }

    Logger::Impl::Impl(LogLevel level, int savedErrno, const char *file, int line)
        : _stream(),
          _level(level),
          _line(line),
          _basename(file) {
        struct timeval tv;
        gettimeofday(&tv, NULL);
        time_t seconds = tv.tv_sec;
        int microseconds = tv.tv_usec;

        if (seconds != t_last_second) {
            t_last_second = seconds;
            struct tm tm_time;
            ::localtime_r(&seconds, &tm_time);

            int len = snprintf(t_time, sizeof(t_time), "%4d%02d%02d %02d:%02d:%02d",
                               tm_time.tm_year + 1900, tm_time.tm_mon + 1, tm_time.tm_mday,
                               tm_time.tm_hour, tm_time.tm_min, tm_time.tm_sec);
            assert(len == 17);
            (void)len;
        }
        _stream << T(t_time, 17) << "." << microseconds << " ";
        // Wrap thread id logic?
        // _stream << std::this_thread::get_id() << " ";
        _stream << LogLevelName[static_cast<unsigned long>(level)];
        if (savedErrno != 0)
        {
            #if defined(__GLIBC__) && defined(_GNU_SOURCE)
                char errbuf[128];
                char *msg = strerror_r(savedErrno, errbuf, sizeof(errbuf));
                _stream << msg << " (errno=" << savedErrno << ") ";
            #else
                char errbuf[128];
                int rc = strerror_r(savedErrno, errbuf, sizeof(errbuf));
                (void)rc;
                _stream << errbuf << " (errno=" << savedErrno << ") ";
            #endif
        }
    }

    void Logger::Impl::finish() {
        _stream << " - " << _basename << ':' << _line << '\n';
    }

    Logger::Logger(const char *file, int line, LogLevel level)
        : _impl(level, 0, file, line)
    {
    }

    Logger::Logger(const char *file, int line, bool toAbort)
        : _impl(toAbort ? LogLevel::FATAL : LogLevel::ERROR, errno, file, line)
    {
    }

    Logger::~Logger() {
        _impl.finish();
        const LogStream::Buffer &buf(stream().buffer());
        auto out = g_output.load(std::memory_order_acquire);
        if (out) {
            out(buf.data(), buf.length());
        }
        if (_impl._level == LogLevel::FATAL)
        {
            auto flush = g_flush.load(std::memory_order_acquire);
            if (flush) {
                flush();
            }
            abort();
        }
    }

    LogLevel Logger::logLevel() {
        return g_logLevel.load(std::memory_order_acquire);
    }

    void Logger::setLogLevel(LogLevel level) {
        g_logLevel.store(level,  std::memory_order_release);
    }

    void Logger::setOutput(OutputFunc out) {
        g_output.store(out, std::memory_order_release);
    }

    void Logger::setFlush(FlushFunc flush) {
        g_flush.store(flush, std::memory_order_release);
    }

}
