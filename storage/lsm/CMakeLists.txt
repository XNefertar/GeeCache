cmake_minimum_required(VERSION 3.10)
project(lsm_engine CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# 查找并链接 Snappy (可选的压缩库)
find_package(Snappy)

# 头文件目录
include_directories(include)
include_directories(.)

# 源代码
file(GLOB_RECURSE SOURCES "core/*.cc" "util/*.cc")

# 添加库
add_library(lsm STATIC ${SOURCES})

# 如果找到 Snappy，则链接它
if(SNAPPY_FOUND)
    target_include_directories(lsm PRIVATE ${SNAPPY_INCLUDE_DIR})
    target_link_libraries(lsm PRIVATE ${SNAPPY_LIBRARY})
endif()

# 设置输出目录
set(CMAKE_ARCHIVE_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR})
