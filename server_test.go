package geecache

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

// 模拟数据库
var serverDB = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

func createGroup() *Group {
	return NewGroup("scores", 2<<10, GetterFunc(
		func(key string) ([]byte, error) {
			log.Println("[SlowDB] search key", key)
			if v, ok := serverDB[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}))
}

func startCacheServer(addr string, addrs []string, gee *Group) {
	peers := NewHTTPPool(addr)
	peers.Set(addrs...)
	gee.RegisterPeers(peers)
	log.Println("geecache is running at", addr)
	// addr[7:] 是为了去掉 "http://"
	log.Fatal(http.ListenAndServe(addr[7:], peers))
}

func startAPIServer(apiAddr string, gee *Group) {
	http.Handle("/api", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			key := r.URL.Query().Get("key")
			view, err := gee.Get(key)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(view.ByteSlice())

		}))
	log.Println("fontend server is running at", apiAddr)
	log.Fatal(http.ListenAndServe(apiAddr[7:], nil))
}

// TestServer 这是一个特殊的测试函数，用于启动服务器
// 使用方法：通过环境变量控制端口
// PORT=8001 go test -v -run TestServer
func TestServer(t *testing.T) {
	// 1. 获取环境变量中的端口，默认为 8001
	portStr := os.Getenv("PORT")
	port := 8001
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err == nil {
			port = p
		}
	}

	// 2. 获取是否启动 API 服务器，默认为 false
	api := false
	if os.Getenv("API") == "1" {
		api = true
	}

	// 3. 固定的节点配置
	apiAddr := "http://localhost:9999"
	addrMap := map[int]string{
		8001: "http://localhost:8001",
		8002: "http://localhost:8002",
		8003: "http://localhost:8003",
	}

	var addrs []string
	for _, v := range addrMap {
		addrs = append(addrs, v)
	}

	// 4. 创建 Group
	gee := createGroup()

	// 5. 如果是 API 节点，启动 API 服务
	if api {
		go startAPIServer(apiAddr, gee)
		// 稍微等待一下，避免日志打印混乱
		time.Sleep(time.Second)
	}

	// 6. 启动缓存服务（这一步是阻塞的，所以最后执行）
	startCacheServer(addrMap[port], addrs, gee)
}
