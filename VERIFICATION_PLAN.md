# GeeCache 完整验证方案

本文档提供了一个全方位的验证方案，用于验证 GeeCache 项目的 C++ 存储引擎、Go 分布式缓存层、前端日志查看器以及整体系统的连通性。

## 1. 验证概览

验证过程分为四个阶段：
1.  **C++ 核心层验证**：验证 LSM 存储引擎和日志系统的正确性。
2.  **Go 单元与集成测试**：验证缓存逻辑、CGO 桥接以及分布式组件。
3.  **系统连通性测试 (E2E)**：手动启动服务，验证 HTTP/gRPC 接口与数据持久化。
4.  **前端与可观测性**：验证 LogViewer 和 Metrics。

---

## 2. 自动化验证脚本

已为您准备了一个自动化脚本 `verify.sh`，它可以一键完成构建和测试工作。

### 使用方法
```bash
chmod +x verify.sh
./verify.sh
```

该脚本将依次执行：
1.  检查构建工具 (CMake, Go, Make)。
2.  构建 C++ `lsm` 静态库 (`liblsm.a`)。
3.  运行 C++ 单元测试 (`lsm_test`)。
4.  运行 Go 语言的所有单元测试 (包含 `-race` 检测)。
5.  构建 Go 的可执行文件 (`geecache-server`, `logviewer` 等)。

---

## 3. 手动分步验证指南

如果您需要深入排查问题或手动验证，请遵循以下步骤。

### 第一阶段：C++ 存储引擎 (L3 Cache)
本项目依赖底层的 C++ LSM Tree 实现。必须通过测试以确保数据不会丢失。

1.  **构建 C++ 库**
    ```bash
    cd cpp/lsm
    mkdir -p build && cd build
    cmake ..
    make -j4
    ```
    *预期结果*：在 `build` 目录下生成 `liblsm.a`。

2.  **运行 C++ 测试**
    ```bash
    ./lsm_test
    ```
    *预期结果*：输出 `All tests passed` 或具体的测试用例成功信息。

### 第二阶段：Go 核心与 CGO 桥接
验证 Go 语言层能否正确调用 C++ 库，以及分布式逻辑是否正常。

1.  **准备工作**
    确保在 `GeeCache` 根目录下。

2.  **运行 Go 测试**
    ```bash
    cd go
    go test -v -race ./...
    ```
    特别关注 `go/bridge/lsm_test.go`，它验证了 Go 到 C++ 的调用路径。

### 第三阶段：系统连通性与数据链路 (前端 + 后端)

我们将启动一个简单的缓存节点，并通过 HTTP 接口进行读写。

1.  **启动后端服务 (GeeCache Server)**
    打开终端 A，运行：
    ```bash
    # 假设已在 verify.sh 中构建完成，二进制在 bin/ 目录
    # 或者直接运行源码：
    cd go/cmd/geecache-server
    go run main.go -port 8001 -group scores -cacheBytes 10000000
    ```
    *日志应显示*：`Starting independent GeeCache server at 0.0.0.0:8001...`

2.  **数据写入验证 (模拟 Client)**
    打开终端 B，使用 `curl` 写入数据：
    ```bash
    # 注意：geecachehttp默认仅支持 GET，若要支持 SET 需要开启 DirectSet (源码中已包含 PUT 处理)
    curl -X PUT -d "123456" http://localhost:8001/_geecache/scores/Tom
    ```
    *预期结果*：HTTP 200 OK。

3.  **数据读取验证**
    读取刚才写入的数据：
    ```bash
    curl http://localhost:8001/_geecache/scores/Tom
    ```
    *预期结果*：返回 `123456`。

4.  **前端页面 (LogViewer) 验证**
    LogViewer 是一个 Web 界面，用于查看服务器日志。
    
    *   **生成日志**：确保之前的 Server 生成了日志文件（如果 Server 输出到 Stdout，可能需要重定向：`go run main.go ... > server.log 2>&1`）。
    *   **启动 LogViewer**：
        ```bash
        cd go/cmd/logviewer
        go run main.go -log-dir . -port 8080
        ```
    *   **访问页面**：
        在浏览器或使用 curl 访问 `http://localhost:8080`。
        由于环境限制，您可能只能看到 HTML 源码。

### 第四阶段：多节点分布式验证

验证节点间通信。

1.  **启动 3 个节点**
    需要修改启动代码或使用具体的集成测试脚本来让节点互相知晓（当前 `geecache-server` main.go 默认为独立模式）。
    建议直接运行集成测试来验证分布式逻辑：
    ```bash
    cd go/tests
    go test -v -run TestServer
    ```
    该测试会自动启动 8001 端口的节点并模拟请求。

---

## 常见问题排查

*   **CGO 报错 `ld: library not found for -llsm`**：
    *   确认 C++ 库已编译。
    *   确认 `go/bridge/lsm.go` 中的 `LDFLAGS` 路径指向正确的 `cpp/lsm/build` 目录。

*   **Go Test 超时**：
    *   可能是死锁。请确保测试命令加上 `-race` 进行检测。
