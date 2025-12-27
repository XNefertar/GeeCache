# Build Stage
FROM gcc:12 AS builder

# Install CMake
RUN apt-get update && apt-get install -y cmake

WORKDIR /app

# Copy source code
COPY . .

# Build
RUN rm -rf build && mkdir build && cd build && \
    cmake -DCMAKE_BUILD_TYPE=Release .. && \
    make storage_server

# Runtime Stage
FROM debian:bookworm-slim

WORKDIR /app

# Install runtime dependencies if any (libstdc++ is usually included)
# Clean up apt cache to keep image small
RUN apt-get update && apt-get install -y libstdc++6 && rm -rf /var/lib/apt/lists/*

# Copy executable from builder
COPY --from=builder /app/build/server/storage_server .

# Create data directory
RUN mkdir -p /data

# Expose RPC port
EXPOSE 9000

# Run server
# Usage: ./storage_server <port> <db_path>
CMD ["./storage_server", "9000", "/data/lsm_db"]
