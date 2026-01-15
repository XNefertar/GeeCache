
namespace lsm {
    class LogSink {
    public:
        virtual void Write(const char* data, size_t len) = 0;
        virtual void Flush() = 0;
        virtual ~LogSink() = default;
    };
}