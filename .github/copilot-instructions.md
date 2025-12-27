# GeeCache Copilot Instructions

## Project Overview
GeeCache is a hybrid distributed cache system combining a Go-based distributed cache layer with a C++ LSM-tree storage engine.
- **Go Layer**: Handles distributed consensus, HTTP transport, LRU caching (L1/L2), and client interactions.
- **C++ Layer**: Implements a Log-Structured Merge (LSM) tree for persistent L3 storage.
- **Bridge**: Uses CGO to interface between Go and the C++ static library.

## Architecture & Data Flow
- **Core Cache (`go/geecache.go`)**: Manages `Group`s, which contain `mainCache` (L1), `hotCache` (L2), and `centralCache` (L3).
- **L3 Storage (`go/bridge/lsm.go`)**: The `LSMStore` struct implements the `CentralCache` interface, wrapping the C++ `lsm` library.
- **Distribution**: `PeerPicker` and `consistenthash` determine which node owns a key.
- **Transport**: `geecachehttp` implements HTTP communication between peers using Protobuf (`geecachepb`).

## Build & Test Workflow
**CRITICAL**: The C++ library must be built *before* running any Go code or tests, as Go depends on `liblsm.a`.

### 1. Build C++ Storage Engine
```bash
cd storage/lsm
mkdir -p build && cd build
cmake ..
make
# Output: storage/lsm/build/liblsm.a
```

### 2. Run Go Tests
```bash
cd go
# -race is mandatory for detecting concurrency issues in the distributed cache
go test -v -race ./...
```

## Code Conventions & Patterns

### Go (Distributed Layer)
- **Interfaces**: Use `Getter`, `Setter`, and `CentralCache` interfaces to decouple components.
- **Concurrency**: Use `singleflight` (`go/singleflight`) to prevent cache stampedes.
- **CGO Bridge**:
  - Keep CGO code isolated in `go/bridge/`.
  - `LSMStore` must manage C memory manually (`C.CString`, `C.free`).
  - Link flags are defined in `go/bridge/lsm.go`: `#cgo LDFLAGS: -L../../storage/lsm/build -llsm -lstdc++`.

### C++ (Storage Layer)
- **Standard**: C++17.
- **Build System**: CMake.
- **Structure**:
  - `core/`: Internal implementation (`memtable`, `wal`, `sstable`).
  - `include/`: Public API headers (`lsm.h`).
  - `util/`: Shared utilities (`skiplist`).
- **Memory Management**: Follow RAII patterns where possible, but expose C-compatible APIs in `lsm.h` for CGO.

## Key Files
- `go/geecache.go`: Main `Group` logic and cache coordination.
- `go/bridge/lsm.go`: CGO implementation of `CentralCache`.
- `storage/lsm/include/lsm.h`: Public C API for the storage engine.
- `storage/lsm/core/db.cc`: Core LSM database logic.
- `go/geecachehttp/http.go`: HTTP transport implementation.
