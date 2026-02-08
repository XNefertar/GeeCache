# GeeCache C++ 接入指南

GeeCache 是一个基于 Go 语言开发的分布式缓存系统。虽然核心逻辑运行在 Go 运行时中，但它提供了标准化的通信协议接口，完全支持跨语言调用。

对于 C++ 项目，这里提供两种标准的接入方式：

1.  **gRPC 接入 (推荐)**：性能更高，类型安全，适合高频调用。
2.  **HTTP 接入**：简单通用，依赖少，适合轻量级集成。

---

## 方案一：gRPC 接入 (高性能推荐)

GeeCache 使用 Protobuf 定义了服务接口。你可以通过 `.proto` 文件生成 C++ 客户端代码。

### 1. 准备工作

你需要安装 `protoc` 编译器和 gRPC C++ 插件。

### 2. 获取 Proto 文件

GeeCache 的接口定义文件位于：`proto/geecache.proto`。

```protobuf
syntax = "proto3";

package geecachepb;

option go_package = "geecache/geecachepb";

message Request {
  string group = 1;
  string key = 2;
}

message Response {
  bytes value = 1;
}

service GroupCache {
  rpc Get(Request) returns (Response);
  rpc Remove(Request) returns (Response);
}
```

### 3. 生成 C++ 代码

在项目根目录下执行以下命令:

> **注意**: 确保在仓库根目录执行,并且已安装 `protoc` 和 `grpc_cpp_plugin`

```bash
mkdir -p cpp_client/generated
protoc --cpp_out=cpp_client/generated \
       --grpc_out=cpp_client/generated \
       --plugin=protoc-gen-grpc=`which grpc_cpp_plugin` \
       proto/geecache.proto
```

这将生成 `geecache.pb.h`, `geecache.pb.cc`, `geecache.grpc.pb.h`, `geecache.grpc.pb.cc` 四个文件。

### 4. 编写 C++ 客户端代码

```cpp
#include <iostream>
#include <memory>
#include <string>

#include <grpcpp/grpcpp.h>
#include "generated/geecache.grpc.pb.h"

using grpc::Channel;
using grpc::ClientContext;
using grpc::Status;
using geecachepb::GroupCache;
using geecachepb::Request;
using geecachepb::Response;

class GeeCacheClient {
 public:
  GeeCacheClient(std::shared_ptr<Channel> channel)
      : stub_(GroupCache::NewStub(channel)) {}

  std::string Get(const std::string& group, const std::string& key) {
    Request request;
    request.set_group(group);
    request.set_key(key);

    Response reply;
    ClientContext context;

    Status status = stub_->Get(&context, request, &reply);

    if (status.ok()) {
      return reply.value();
    } else {
      std::cout << status.error_code() << ": " << status.error_message()
                << std::endl;
      return "RPC failed";
    }
  }

 private:
  std::unique_ptr<GroupCache::Stub> stub_;
};

int main(int argc, char** argv) {
  // 假设 Go Server 运行在 localhost:9999
  GeeCacheClient client(grpc::CreateChannel(
      "localhost:9999", grpc::InsecureChannelCredentials()));
  
  std::string user = client.Get("users", "1001");
  std::cout << "GeeCache received: " << user << std::endl;

  return 0;
}
```

---

## 方案二：HTTP 接入 (简单通用)

如果不想引入 gRPC 复杂的依赖，可以直接使用 HTTP 请求。GeeCache 暴露了标准的 RESTful 接口。

**接口格式：** `http://<host>:<port>/_geecache/<group>/<key>`

### 使用 libcurl 示例

```cpp
#include <iostream>
#include <string>
#include <curl/curl.h>

// 用于接收 HTTP 响应数据的回调函数
size_t WriteCallback(void* contents, size_t size, size_t nmemb, void* userp) {
    ((std::string*)userp)->append((char*)contents, size * nmemb);
    return size * nmemb;
}

int main() {
    CURL* curl;
    CURLcode res;
    std::string readBuffer;

    curl = curl_easy_init();
    if(curl) {
        // 构建请求 URL: http://localhost:9999/_geecache/users/1001
        std::string url = "http://localhost:9999/_geecache/users/1001";
        
        curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, WriteCallback);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, &readBuffer);
        
        res = curl_easy_perform(curl);
        
        if(res != CURLE_OK) {
            fprintf(stderr, "curl_easy_perform() failed: %s\n",
              curl_easy_strerror(res));
        } else {
            long http_code = 0;
            curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &http_code);
            if (http_code == 200) {
                 std::cout << "Cache Hit: " << readBuffer << std::endl;
            } else {
                 std::cout << "Cache Miss or Error (HTTP " << http_code << ")" << std::endl;
            }
        }
        
        curl_easy_cleanup(curl);
    }
    return 0;
}
```

---

## 架构说明

### Sidecar 模式 (如果你想实现类似 Cache-Aside)

由于你的 C++ 应用无法直接向 Go 的 Group 中注入 `Getter` 回调（跨进程内存不共享），通常采用 **Sidecar (旁路缓存)** 模式：

1.  **架构部署**：部署一个独立的 Go GeeCache 服务进程。
2.  **数据写入**：
    *   C++ 服务先写 DB。
    *   C++ 服务直接调用 GeeCache 的 API (Set/Delete) 来更新或删除缓存。
    *   *(注：目前 GeeCache HTTP 接口主要暴露 Get，如果需要 Sidecar 模式，建议在 Go 端扩展 Put/Delete 接口)*
3.  **数据读取**：
    *   C++ 服务请求 GeeCache (HTTP/gRPC)。
    *   如果返回 200，直接使用。
    *   如果返回 404，C++ 服务自己查 DB。

### 独立 Go 服务启动代码

你需要编译并运行一个独立的 Go 服务供 C++ 调用：

```go
package main

import (
    "geecache"
    "geecache/geecachegrpc"
    "net"
    "log"
    "google.golang.org/grpc"
)

func main() {
    // 创建一个 Cache Group
    // 在 Sidecar 模式下，Getter 可能设为 nil (完全由客户端控制写入) 
    // 或者实现一个通用的 HTTP 回调去问 C++ 服务 (比较复杂，不推荐)
    geecache.NewGroup("users", 2<<30, geecache.GetterFunc(
        func(key string) ([]byte, error) {
            return nil, fmt.Errorf("key not found in cache")
        }))

    // 启动 gRPC 服务
    lis, _ := net.Listen("tcp", ":9999")
    s := grpc.NewServer()
    geecachegrpc.NewGroupCacheServer(s) 
    s.Serve(lis)
}
```
