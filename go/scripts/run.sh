#!/bin/bash

# 遇到错误立即停止脚本执行
set -e

# 获取当前脚本所在目录
DIR=$(cd "$(dirname "$0")"; pwd)
echo "Current Directory: $DIR"

# Ensure we are in the go module root
cd "$DIR/.."

echo "------------------------------------------------"
echo "1. Formatting code (go fmt)..."
# 格式化所有代码，保持风格统一
go fmt ./...

echo "------------------------------------------------"
echo "2. Static Analysis (go vet)..."
# 静态分析，检查代码中潜在的错误（比如 printf 参数不对，锁复制等）
go vet ./...

echo "------------------------------------------------"
echo "3. Running Tests with Race Detector..."
# 核心测试命令：
# -v: 显示详细日志 (verbose)
# -race: 开启竞态检测（非常重要！专门检测并发读写冲突）
# ./...: 递归测试当前目录下所有包
go test -v -race ./...

echo "------------------------------------------------"
echo "All tests passed successfully!"