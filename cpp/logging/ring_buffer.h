namespace lsm {
    template <int SIZE>
    class FixedBuffer {
    public:
        FixedBuffer() : _cur(_data) {}
        ~FixedBuffer() = default;

        FixedBuffer(const FixedBuffer&) = delete;
        FixedBuffer& operator=(const FixedBuffer&) = delete;

        void append(const char* buf, size_t len) {
            if (avail() > len) {
                memcpy(_cur, buf, len);
                _cur += len;
            }
        }

        const char* data() const { return _data; }
        int length() const { return static_cast<int>(_cur - _data); }
        void reset() { _cur = _data; }
        int avail() const { return static_cast<int>(end() - _cur); }

    private:
        char _data[SIZE];
        char* _cur;
        const char* end() const { return _data + SIZE; }
    };
}