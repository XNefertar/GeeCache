#include "logging.h"

#include <cerrno>
#include <cstdio>
#include <cstring>
#include <ctime>
#include <sys/time.h>
#include <thread>
#include <cassert>

namespace lsm
{

    __thread char t_time[64];
    __thread time_t t_last_second;

    void defaultOutput(const char *msg, int len)
    {
        size_t n = fwrite(msg, 1, len, stdout);
        (void)n;
    }

    void defaultFlush()
    {
        fflush(stdout);
    }

    Logger::OutputFunc g_output = defaultOutput;
    Logger::FlushFunc g_flush = defaultFlush;

    LogLevel g_logLevel = INFO;

    const char *LogLevelName[NUM_LOG_LEVELS] =
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
        _stream << LogLevelName[level];
        if (savedErrno != 0)
        {
            _stream << strerror_r(savedErrno, t_time, sizeof(t_time)) << " (errno=" << savedErrno << ") ";
        }
    }

    void Logger::Impl::finish()
    {
        _stream << " - " << _basename << ':' << _line << '\n';
    }

    Logger::Logger(const char *file, int line, LogLevel level)
        : _impl(level, 0, file, line)
    {
    }

    Logger::Logger(const char *file, int line, bool toAbort)
        : _impl(toAbort ? FATAL : ERROR, errno, file, line)
    {
    }

    Logger::~Logger()
    {
        _impl.finish();
        const LogStream::Buffer &buf(stream().buffer());
        g_output(buf.data(), buf.length());
        if (_impl._level == FATAL)
        {
            g_flush();
            abort();
        }
    }

    LogLevel Logger::logLevel()
    {
        return g_logLevel;
    }

    void Logger::setLogLevel(LogLevel level)
    {
        g_logLevel = level;
    }

    void Logger::setOutput(OutputFunc out)
    {
        g_output = out;
    }

    void Logger::setFlush(FlushFunc flush)
    {
        g_flush = flush;
    }

}
