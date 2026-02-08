find_package(Threads REQUIRED)

add_executable(geecache_server main.cc)
target_link_libraries(geecache_server PRIVATE lsm logging Threads::Threads)
target_include_directories(geecache_server PRIVATE ../rpc)

add_executable(client_test client_test.cc)
target_link_libraries(client_test PRIVATE Threads::Threads)
target_include_directories(client_test PRIVATE ../rpc)

add_executable(storage_server storage_server.cc)
target_link_libraries(storage_server PRIVATE lsm logging Threads::Threads)
target_include_directories(storage_server PRIVATE ../rpc)

add_executable(simple_client simple_client.cc)
target_link_libraries(simple_client PRIVATE Threads::Threads)
target_include_directories(simple_client PRIVATE ../rpc)
