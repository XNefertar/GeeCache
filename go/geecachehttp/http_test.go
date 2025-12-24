package geecachehttp

import (
	"fmt"
	"geecache"
	pb "geecache/geecachepb"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestHTTPPoolServeHTTP(t *testing.T) {
	db := map[string]string{
		"Tom":  "630",
		"Jack": "589",
	}

	groupName := "scores_test"
	_, err := geecache.NewGroup(groupName, 2<<10, geecache.GetterFunc(
		func(key string) ([]byte, error) {
			t.Logf("[MockDB] searching key %s", key)
			if v, ok := db[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		},
	))
	if err != nil {
		t.Fatal(err)
	}

	pool := NewHTTPPool("localhost:test")

	validPath := fmt.Sprintf("/_geecache/%s/%s", groupName, "Tom")
	req := httptest.NewRequest("GET", validPath, nil)

	w := httptest.NewRecorder()

	pool.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Case 1 失败: 期望状态码 200, 但得到 %d", w.Code)
	}
	// 5.2 验证 Body 内容 (应该是 "630")
	res := &pb.Response{}
	if err := proto.Unmarshal(w.Body.Bytes(), res); err != nil {
		t.Errorf("proto unmarshal failed: %v", err)
	}
	if string(res.Value) != "630" {
		t.Errorf("Case 1 失败: 期望 Body '630', 但得到 '%s'", string(res.Value))
	}
	// 5.3 验证 Content-Type
	if contentType := w.Header().Get("Content-Type"); contentType != "application/octet-stream" {
		t.Errorf("Case 1 失败: 期望 Content-Type 为 application/octet-stream, 但得到 %s", contentType)
	}

	invalidPath := fmt.Sprintf("/_geecache/%s/%s", groupName, "UnknownKey")
	reqErr := httptest.NewRequest("GET", invalidPath, nil)
	wErr := httptest.NewRecorder()

	pool.ServeHTTP(wErr, reqErr)
	if wErr.Code != http.StatusInternalServerError {
		t.Errorf("Case 2 失败: Key 不存在时期望状态码 500, 但得到 %d", wErr.Code)
	}

	if !strings.Contains(wErr.Body.String(), "not exist") {
		t.Errorf("Case 2 失败: 期望错误信息包含 'not exist', 但得到 '%s'", wErr.Body.String())
	}
}
