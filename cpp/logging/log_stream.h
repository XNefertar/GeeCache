#pragma once

#include <string>
#include <cstring>
#include "ring_buffer.h"

namespace lsm
{
    class T;
    class LogStream
    {
    public:
        using Buffer = FixedBuffer<4000>; // Small buffer for a single log message

        LogStream() = default;
        LogStream(const LogStream &) = delete;
        LogStream &operator=(const LogStream &) = delete;

        LogStream &operator<<(bool v) {
            _buffer.append(v ? "1" : "0", 1);
            return *this;
        }

        LogStream &operator<<(short);
        LogStream &operator<<(unsigned short);
        LogStream &operator<<(int);
        LogStream &operator<<(unsigned int);
        LogStream &operator<<(long);
        LogStream &operator<<(unsigned long);
        LogStream &operator<<(long long);
        LogStream &operator<<(unsigned long long);
        LogStream &operator<<(const void *);
        LogStream &operator<<(float v) {
            *this << static_cast<double>(v);
            return *this;
        }
        LogStream &operator<<(double);
        LogStream &operator<<(char v) {
            _buffer.append(&v, 1);
            return *this;
        }
        LogStream &operator<<(const char *str) {
            if (str) {
                _buffer.append(str, strlen(str));
            }
            else {
                _buffer.append("(null)", 6);
            }
            return *this;
        }
        LogStream &operator<<(const std::string &v) {
            _buffer.append(v.c_str(), v.size());
            return *this;
        }
        LogStream &operator<<(const T &t);

        const Buffer &buffer() const { return _buffer; }
        void resetBuffer() { _buffer.reset(); }

    private:
        void staticCheck();

        template <typename T>
        void formatInteger(T);

        Buffer _buffer;
        static const int kMaxNumericSize = 32;
    };

}
