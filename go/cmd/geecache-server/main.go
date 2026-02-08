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
	var groupName string
	var cacheBytes int64
	var advertiseAddr string

	flag.IntVar(&port, "port", 9999, "GeeCache server port")
	flag.StringVar(&groupName, "group", "default", "Cache group name")
	flag.Int64Var(&cacheBytes, "cacheBytes", 1<<30, "Cache capacity in bytes")
	flag.StringVar(&advertiseAddr, "advertise", "", "Advertised address for peer communication (default: same as server address)") // 新增 advertise 参数
	flag.Parse()

	// 监听地址 (注意：在容器或云环境中可能需要改为 0.0.0.0)
	listenAddr := fmt.Sprintf("0.0.0.0:%d", port)
	if advertiseAddr == "" {
		advertiseAddr = fmt.Sprintf("localhost:%d", port)
	}

	// 初始化 HTTP Pool
	// self 参数主要用于节点间通信时的标识，这里直接用地址
	peers := geecachehttp.NewHTTPPool(advertiseAddr)

	_, err := geecache.NewGroup(groupName, cacheBytes, nil)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Starting independent GeeCache server at %s...", listenAddr)
	log.Printf(" - Advertise address: %s", advertiseAddr)
	log.Printf(" - Cache Group: '%s'", groupName)
	log.Printf(" - Max Bytes:   %d bytes", cacheBytes)
	log.Printf(" - Access URL:  http://localhost:%d/_geecache/%s/<key>", port, groupName)

	// 启动 HTTP 服务
	// peers (HTTPPool) 实现了 ServeHTTP 接口
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), peers))
}
