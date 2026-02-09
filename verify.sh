#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Starting GeeCache Complete Verification ===${NC}"

PROJECT_ROOT=$(pwd)
echo "Project Root: $PROJECT_ROOT"

# 1. Check Prerequisites
echo -e "\n${GREEN}[Step 1] Checking Build Tools...${NC}"
command -v cmake >/dev/null 2>&1 || { echo -e "${RED}Error: cmake is required but not installed.${NC}"; exit 1; }
command -v go >/dev/null 2>&1 || { echo -e "${RED}Error: go is required but not installed.${NC}"; exit 1; }
command -v make >/dev/null 2>&1 || { echo -e "${RED}Error: make is required but not installed.${NC}"; exit 1; }
echo "All tools found."

# 2. Build C++ Core
echo -e "\n${GREEN}[Step 2] Building C++ Storage Engine (LSM Core)...${NC}"
cd "$PROJECT_ROOT/cpp/lsm"
mkdir -p build
cd build
cmake ..
make -j$(nproc)

if [ ! -f "liblsm.a" ]; then
    echo -e "${RED}Build failed: liblsm.a not found!${NC}"
    exit 1
fi
echo "C++ build successful."

# 3. Run C++ Tests
echo -e "\n${GREEN}[Step 3] Running C++ Unit Tests...${NC}"
# Check if tests exist
if [ -f "./lsm_test" ]; then
    ./lsm_test
    echo "C++ tests passed."
else
    echo -e "${RED}Warning: lsm_test executable not found.${NC}"
fi

# 4. Run Go Tests
echo -e "\n${GREEN}[Step 4] Running Go Unit & Integration Tests...${NC}"
cd "$PROJECT_ROOT/go"
# Run bridge tests specifically first (Critical path)
echo "Testing CGO Bridge..."
go test -v -race ./bridge/...

echo "Testing All Go Modules..."
go test -v -race ./...

# 5. Build Go Executables
echo -e "\n${GREEN}[Step 5] Building Go Binaries...${NC}"
mkdir -p "$PROJECT_ROOT/bin"

echo "Building geecache-server..."
cd "$PROJECT_ROOT/go/cmd/geecache-server"
go build -o "$PROJECT_ROOT/bin/geecache-server" main.go

echo "Building logviewer..."
cd "$PROJECT_ROOT/go/cmd/logviewer"
go build -o "$PROJECT_ROOT/bin/logviewer" main.go

echo -e "\n${GREEN}=== Verification Complete! ===${NC}"
echo "Binaries are located in '$PROJECT_ROOT/bin/'"
echo "See VERIFICATION_PLAN.md for manual running instructions."
