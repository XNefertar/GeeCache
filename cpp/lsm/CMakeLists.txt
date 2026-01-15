cmake_minimum_required(VERSION 3.10)
project(lsm_engine CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# 查找并链接 Snappy (可选的压缩库)
find_package(Snappy)

# 源代码
file(GLOB_RECURSE SOURCES "core/*.cc" "util/*.cc")

# 添加库
add_library(lsm STATIC ${SOURCES})

# 头文件目录：通过目标接口公开给下游目标
target_include_directories(lsm PUBLIC
    ${CMAKE_CURRENT_SOURCE_DIR}/include
    ${CMAKE_CURRENT_SOURCE_DIR}
)

find_package(Threads REQUIRED)
target_link_libraries(lsm PRIVATE Threads::Threads)

# 如果找到 Snappy，则链接它
if(SNAPPY_FOUND)
    target_include_directories(lsm PRIVATE ${SNAPPY_INCLUDE_DIR})
    target_link_libraries(lsm PRIVATE ${SNAPPY_LIBRARY})
endif()

# 设置输出目录
set(CMAKE_ARCHIVE_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR})

# Tests
add_executable(lsm_test test/lsm_test.cc)
target_link_libraries(lsm_test PRIVATE lsm Threads::Threads)

# Benchmarks
add_executable(lsm_bench test/lsm_bench.cc)
target_link_libraries(lsm_bench PRIVATE lsm Threads::Threads)

# TokenBucket Test
add_executable(token_bucket_test test/token_bucket_test.cc)
target_link_libraries(token_bucket_test PRIVATE lsm Threads::Threads)

# DB Rate Limit Test
add_executable(db_ratelimit_test test/db_ratelimit_test.cc)
target_link_libraries(db_ratelimit_test PRIVATE lsm Threads::Threads)

