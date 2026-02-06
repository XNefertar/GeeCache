cmake_minimum_required(VERSION 3.10)
project(geecache_logging LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

file(GLOB LOGGING_IMPL ${CMAKE_CURRENT_SOURCE_DIR}/impl/*.cc)
file(GLOB LOGGING_SINKS ${CMAKE_CURRENT_SOURCE_DIR}/sinks/*.cc)

set(LOGGING_SOURCES
    ${CMAKE_CURRENT_SOURCE_DIR}/logging.cc
    ${CMAKE_CURRENT_SOURCE_DIR}/async_logging.cc
    ${CMAKE_CURRENT_SOURCE_DIR}/log_stream.cc
)

list(APPEND LOGGING_SOURCES ${LOGGING_IMPL} ${LOGGING_SINKS})

add_library(logging STATIC ${LOGGING_SOURCES})

target_include_directories(logging
    PUBLIC
        ${CMAKE_CURRENT_SOURCE_DIR}
)

if(UNIX)
    find_package(Threads REQUIRED)
    target_link_libraries(logging PUBLIC Threads::Threads)
endif()

# Benchmark executable
add_executable(bench_logging bench_logging.cc)
target_link_libraries(bench_logging PRIVATE logging Threads::Threads)


target_compile_definitions(logging PUBLIC -DLSM_LOGGING)

install(TARGETS logging
    EXPORT geecacheTargets
    ARCHIVE DESTINATION lib
    LIBRARY DESTINATION lib
    RUNTIME DESTINATION bin
)

install(FILES
    ${CMAKE_CURRENT_SOURCE_DIR}/logging.h
    ${CMAKE_CURRENT_SOURCE_DIR}/ring_buffer.h
    ${CMAKE_CURRENT_SOURCE_DIR}/async_logging.h
    ${CMAKE_CURRENT_SOURCE_DIR}/log_stream.h
    DESTINATION include/geecache/logging
)

export(EXPORT geecacheTargets FILE ${CMAKE_CURRENT_BINARY_DIR}/geecache_loggingTargets.cmake)

option(BUILD_LOGGING_EXAMPLE "Build logging example and test" OFF)
if(BUILD_LOGGING_EXAMPLE)
    add_executable(logging_example ${CMAKE_CURRENT_SOURCE_DIR}/example/logging_example.cc)
    target_link_libraries(logging_example PRIVATE logging)
endif()