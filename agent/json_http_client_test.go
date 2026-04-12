package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestJSONHTTPClientRegister 验证本地 HTTP client 能把 register 正确发往 sidecar-agent。
func TestJSONHTTPClientRegister(t *testing.T) {
	// 记录是否命中 register 路由，便于断言请求是否真正发送。
	called := false
	// 创建一个最小测试服务端，模拟 sidecar-agent 管理接口。
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// 仅允许命中 register 路由。
		if request.Method != http.MethodPost || request.URL.Path != "/register" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		// 命中正确路由后把标记置为 true。
		called = true
		// 返回 200，模拟 sidecar-agent 成功处理注册。
		writer.WriteHeader(http.StatusOK)
	}))
	// 在测试结束时关闭服务端。
	defer server.Close()
	// 创建待测 client。
	client := NewJSONHTTPClient(server.URL, time.Second)
	// 发起一次注册请求。
	if err := client.Register(context.Background(), RegisterRequest{
		Name: "auth",
		Port: 9090,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	// 注册请求应真正命中服务端。
	if !called {
		t.Fatal("expected register handler to be called")
	}
}
