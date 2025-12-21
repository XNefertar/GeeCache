### 第一阶段：启动 3 个缓存节点

我们需要启动 3 个节点（8001, 8002, 8003），其中 8003 兼职作为 API 接口入口（9999端口）。

#### 终端 1（模拟节点 8001）
打开第一个终端，进入项目根目录，运行：

*   **Mac / Linux:**
    ```bash
    PORT=8001 go test -v -run TestServer
    ```
*   **Windows (PowerShell):**
    ```powershell
    $env:PORT=8001; go test -v -run TestServer
    ```

> **预期结果**：终端卡住（正在运行），打印出 `geecache is running at http://localhost:8001`。

#### 终端 2（模拟节点 8002）
打开第二个终端，运行：

*   **Mac / Linux:**
    ```bash
    PORT=8002 go test -v -run TestServer
    ```
*   **Windows (PowerShell):**
    ```powershell
    $env:PORT=8002; go test -v -run TestServer
    ```

> **预期结果**：终端卡住，打印出 `geecache is running at http://localhost:8002`。

#### 终端 3（模拟节点 8003 + API 入口）
打开第三个终端，运行（注意这里多了 `API=1`）：

*   **Mac / Linux:**
    ```bash
    PORT=8003 API=1 go test -v -run TestServer
    ```
*   **Windows (PowerShell):**
    ```powershell
    $env:PORT=8003; $env:API=1; go test -v -run TestServer
    ```

> **预期结果**：
> 1. 打印 `fontend server is running at http://localhost:9999`
> 2. 打印 `geecache is running at http://localhost:8003`

---

### 第二阶段：执行测试与观察结果

现在你的集群已经启动了。打开浏览器（或者第 4 个终端用 curl），我们要开始访问了。

#### 测试 1：第一次访问（缓存击穿 -> 查库 -> 回填）

**操作**：
在浏览器访问：`http://localhost:9999/api?key=Tom`

**预期现象**：
1.  **浏览器**：显示 `630`。
2.  **终端观察**：
    *   你会发现 **某一个** 终端（可能是 8001，也可能是 8002 或 8003，取决于哈希算法）输出了日志：
        ```text
        [SlowDB] search key Tom
        ```
    *   这就证明：缓存里没有 "Tom"，系统去调用了回调函数（查库）了。

#### 测试 2：第二次访问（命中缓存）

**操作**：
再次刷新浏览器：`http://localhost:9999/api?key=Tom`

**预期现象**：
1.  **浏览器**：依然显示 `630`（速度很快）。
2.  **终端观察**：
    *   **没有任何一个终端** 再次打印 `[SlowDB] search key Tom`。
    *   这就证明：数据是从缓存内存里直接读取的，没有去查库。

#### 测试 3：分布式节点转发

**操作**：
在浏览器访问：`http://localhost:9999/api?key=Jack` （换个 key）

**预期现象**：
1.  **浏览器**：显示 `589`。
2.  **终端观察**：
    *   关注 **终端 3**（也就是启动 API 9999 的那个），它的日志里可能会出现：
        ```text
        Pick peer http://localhost:8001
        ```
        (或者 8002/8003)
    *   这说明：用户请求打到了 9999 端口，9999 计算出 "Jack" 应该归 8001 管，于是它**转发 HTTP 请求**给了 8001。
    *   然后你会看到 **终端 1 (8001)** 打印出了 `[SlowDB] search key Jack`。

---

### 总结：如何判断成功？

只要满足以下三点，你的分布式缓存就写成功了：

1.  **能拿到数据**：浏览器能看到 `630`。
2.  **缓存生效**：第二次刷新时，不再打印 `SlowDB` 日志。
3.  **分布式生效**：能在日志中看到 `Pick peer ...`，说明节点之间发生了通信。