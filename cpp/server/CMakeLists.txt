add_executable(geecache_server main.cc)
target_link_libraries(geecache_server PRIVATE lsm pthread)
target_include_directories(geecache_server PRIVATE ../rpc ../lsm/include)

add_executable(client_test client_test.cc)
target_link_libraries(client_test PRIVATE pthread)
target_include_directories(client_test PRIVATE ../rpc)

add_executable(storage_server storage_server.cc)
target_link_libraries(storage_server PRIVATE lsm pthread)
target_include_directories(storage_server PRIVATE ../rpc ../lsm/include)

