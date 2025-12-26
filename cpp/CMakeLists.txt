cmake_minimum_required(VERSION 3.10)
project(geecache_cpp CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Add the LSM storage engine
add_subdirectory(lsm)
