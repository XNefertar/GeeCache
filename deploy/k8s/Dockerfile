# Build stage for C++ LSM cpp
FROM golang:1.24-bookworm AS cpp-builder

# Install C++ build tools
RUN apt-get update && apt-get install -y \
    cmake \
    g++ \
    make \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy C++ cpp code
COPY cpp/ ./cpp/

# Build LSM cpp library
RUN cd cpp/lsm && \
    mkdir -p build && \
    cd build && \
    cmake .. && \
    make

# Build stage for Go application
FROM golang:1.24-bookworm AS go-builder

ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /build

# Copy go module files
COPY go/go.mod go/go.sum ./go/

# Download dependencies
RUN cd go && go mod download

# Copy C++ build artifacts
COPY --from=cpp-builder /build/cpp/lsm/build/liblsm.a ./cpp/lsm/build/
COPY --from=cpp-builder /build/cpp/lsm/include ./cpp/lsm/include

# Copy Go source
COPY go/ ./go/
COPY proto/ ./proto/

# Build Go binary
RUN cd go && \
    CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo \
    -ldflags="-w -s" \
    -o /geecache-server \
    ./cmd/k8s_server/main.go

# Final stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    ca-certificates \
    libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

ARG UID=10001
ARG GID=10001
RUN groupadd --gid $GID appgroup && \
    useradd --uid $UID --gid $GID --create-home appuser

WORKDIR /app

# Copy binary
COPY --from=go-builder --chown=appuser:appgroup /geecache-server /app/

# Create data directory
RUN mkdir -p /data && chown -R appuser:appgroup /data

USER appuser

# Health check
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
    CMD ["/app/geecache-server", "-health-check"]

EXPOSE 8080 9090

ENTRYPOINT ["/app/geecache-server"]
