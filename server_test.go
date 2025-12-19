package geecache

import (
	"fmt"
	"io"
	"log"
	"net/http"
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
	return NewGroup("scores_server", 2<<10, GetterFunc(
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

func TestServer(t *testing.T) {
	// 1. 固定配置
	addrMap := map[int]string{
		8001: "http://localhost:8001", // 我们只测这一个节点
	}

	var addrs []string
	for _, v := range addrMap {
		addrs = append(addrs, v)
	}

	// 2. 创建 Group
	gee := createGroup()

	// 3. 启动缓存服务器 (后台运行)
	go startCacheServer(addrMap[8001], addrs, gee)

	// 4. 等待服务器启动
	time.Sleep(time.Second)

	// 5. 发起请求
	// 注意：这里的 URL 里的 "scores_server" 必须和 createGroup 里的名字一致
	targetURL := "http://localhost:8001/_geecache/scores_server/Tom"

	t.Logf("发起请求: %s", targetURL)
	res, err := http.Get(targetURL)
	if err != nil {
		t.Fatalf("无法连接到缓存服务器: %v", err)
	}
	defer res.Body.Close()

	// 6. 验证结果
	body, _ := io.ReadAll(res.Body)
	t.Logf("服务器响应: %s", string(body))

	if string(body) != "630" {
		t.Errorf("响应错误! 期望: 630, 实际: %s", string(body))
	}
}
