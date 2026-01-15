cmake_minimum_required(VERSION 3.10)
project(geecache_cpp CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Add the LSM storage engine
add_subdirectory(lsm)

# Add the Logging library (make available to lsm/server)
add_subdirectory(logging)

# Add the RPC server (depends on libraries above)
add_subdirectory(server)