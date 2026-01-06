package main

import (
	"context"
	"flag"
	"fmt"
	"geecache"
	"geecache/geecachehttp"
	"geecache/observability"
	"log"
	"net/http"
	"strings"
)

var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

func main() {
	var port int
	var api bool
	flag.IntVar(&port, "port", 8001, "Geecache server port")
	flag.BoolVar(&api, "api", false, "Start a api server?")
	flag.Parse()

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

	// Initialize Tracer
	shutdown := observability.InitTracer("geecache-demo")
	defer shutdown()

	gee := createGroup()
	if api {
		go startAPIServer(apiAddr, gee)
	}

	startCacheServer(addrMap[port], addrs, gee)
}

func createGroup() *geecache.Group {
	g, _ := geecache.NewGroup("scores", 2<<10, geecache.GetterFunc(
		func(ctx context.Context, key string) ([]byte, error) {
			log.Println("[SlowDB] search key", key)
			if v, ok := db[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}))
	return g
}

func getPort(addr string) string {
	// 简单粗暴的方式：假设地址格式是 http://hostname:port
	parts := strings.Split(addr, ":")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	// 如果格式不对，做个兜底（虽然在你的 demo 里不太可能）
	return "8080"
}

func startCacheServer(addr string, addrs []string, gee *geecache.Group) {
	peers := geecachehttp.NewHTTPPool(addr)
	peers.Set(addrs...)
	gee.RegisterPeers(peers)
	log.Println("geecache is running at", addr)
	// log.Fatal(http.ListenAndServe(addr[7:], peers))
	port := getPort(addr)
	log.Fatal(http.ListenAndServe(":"+port, peers))
}

func startAPIServer(apiAddr string, gee *geecache.Group) {
	http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
    <title>GeeCache Demo</title>
    <style>
        body { font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        .container { border: 1px solid #ccc; padding: 20px; border-radius: 5px; }
        input { padding: 8px; width: 200px; margin-right: 10px; }
        button { padding: 8px 15px; background-color: #007bff; color: white; border: none; border-radius: 3px; cursor: pointer; }
        button:hover { background-color: #0056b3; }
        #result { margin-top: 20px; padding: 10px; background-color: #f8f9fa; border-radius: 3px; min-height: 20px; }
        .links { margin-top: 30px; font-size: 0.9em; }
        .links a { margin-right: 15px; color: #666; }
    </style>
</head>
<body>
    <div class="container">
        <h1>GeeCache Query Demo</h1>
        <p>Try keys: <b>Tom</b>, <b>Jack</b>, <b>Sam</b> (or any other to see cache miss)</p>
        <div>
            <input type="text" id="key" placeholder="Enter key..." onkeydown="if(event.key==='Enter') query()">
            <button onclick="query()">Search</button>
        </div>
        <div id="result"></div>
        
        <div class="links">
            <p>Observability Links:</p>
            <a href="http://localhost:9090" target="_blank">Prometheus (Metrics)</a>
            <a href="http://localhost:16686" target="_blank">Jaeger (Traces)</a>
            <a href="http://localhost:3000" target="_blank">Grafana (Dashboards)</a>
            <a href="/metrics" target="_blank">Raw Metrics</a>
        </div>
    </div>

    <script>
        async function query() {
            const key = document.getElementById('key').value;
            if (!key) return;
            
            const resultDiv = document.getElementById('result');
            resultDiv.innerHTML = 'Querying...';
            resultDiv.style.color = '#666';
            
            const start = performance.now();
            try {
                const res = await fetch('/api?key=' + key);
                const text = await res.text();
                const duration = Math.round(performance.now() - start);
                
                if (res.ok) {
                    resultDiv.innerHTML = '<strong>Value:</strong> ' + text + ' <span style="color:#888; font-size:0.8em">(' + duration + 'ms)</span>';
                    resultDiv.style.color = 'green';
                } else {
                    resultDiv.innerHTML = '<strong>Error:</strong> ' + text + ' <span style="color:#888; font-size:0.8em">(' + duration + 'ms)</span>';
                    resultDiv.style.color = 'red';
                }
            } catch (e) {
                resultDiv.innerText = 'Request failed: ' + e.message;
                resultDiv.style.color = 'red';
            }
        }
    </script>
</body>
</html>
		`))
	}))

	http.Handle("/api", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			key := r.URL.Query().Get("key")
			view, err := gee.Get(r.Context(), key)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write(view.ByteSlice())
		}))
	log.Println("fontend server is running at", apiAddr)
	// log.Fatal(http.ListenAndServe(apiAddr[7:], nil))

	port := getPort(apiAddr)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
