package main

import (
	"flag"
	"fmt"
	"geecache"
	"geecache/geecachehttp"
	"log"
	"net/http"
)

func main() {
	var port int
	flag.IntVar(&port, "port", 9999, "GeeCache server port")
	flag.Parse()

	// 监听地址 (注意：在容器或云环境中可能需要改为 0.0.0.0)
	addr := fmt.Sprintf("0.0.0.0:%d", port)

	// 初始化 HTTP Pool
	// self 参数主要用于节点间通信时的标识，这里直接用地址
	peers := geecachehttp.NewHTTPPool(addr)

	// 初始化核心 Cache Group
	// Name: "default"
	// Capacity: 1GB
	// Getter: nil (因为是独立服务，没有业务逻辑，只负责存取)
	_, err := geecache.NewGroup("default", 1<<30, nil)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Starting independent GeeCache server at %s...", addr)
	log.Printf(" - Cache Group: 'default'")
	log.Printf(" - Max Bytes:   1GB")
	log.Printf(" - Access URL:  http://localhost:%d/_geecache/default/<key>", port)

	// 启动 HTTP 服务
	// peers (HTTPPool) 实现了 ServeHTTP 接口
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), peers))
}
