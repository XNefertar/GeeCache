# HTTP 性能压测指南

这个目录包含了对 GeeCache 进行 HTTP 接口压力测试的工具。

## 1. 准备工作

确保你已经安装了 `wrk` 工具。
```bash
sudo apt install wrk  # Debian/Ubuntu
brew install wrk      # MacOS
```

## 2. 启动测试服务器

首先编译并启动包含 MockDB 的测试服务器：

```bash
cd ../..  # 回到 go/ 根目录
go run cmd/http_benchmark/main.go
```
服务器将在 `localhost:9999` 启动。

## 3. 运行压测

打开一个新的终端窗口，使用 `wrk` 配合 Lua 脚本生成随机请求：

```bash
# -t4: 4个线程
# -c100: 100个连接
# -d30s: 持续30秒
# -s cmd/http_benchmark/random.lua: 加载随机生成 Key 的脚本
wrk -t4 -c100 -d30s -s cmd/http_benchmark/random.lua http://localhost:9999
```

## 4. 预期结果

如果缓存预热完成，你应该能看到高达数万的 QPS（取决于机器性能）。
```plaintext
Requests/sec:  37599.41
Transfer/sec:     22.66MB
```
