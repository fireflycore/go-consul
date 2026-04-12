package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestWatchSourceEmitsConnectedAndDisconnected 验证 watch 事件源会在流建立与关闭时输出事件。
func TestWatchSourceEmitsConnectedAndDisconnected(t *testing.T) {
	// 创建一个最小 SSE 服务端，先输出一个事件再主动关闭连接。
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// 仅允许客户端以 GET 方式建立 watch 长连接。
		if request.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", request.Method)
		}
		// 设置 SSE 必需响应头。
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}
		// 先发送一次 connected 事件，再立刻结束本轮响应，模拟 agent 重启。
		_, _ = writer.Write([]byte("event: connected\n"))
		_, _ = writer.Write([]byte("data: ok\n\n"))
		// 立刻刷新到客户端，让事件源能尽快收到 connected。
		flusher.Flush()
	}))
	// 在测试结束时关闭服务端。
	defer server.Close()
	// 创建待测 watch 事件源。
	source := NewWatchSource(server.URL, 10*time.Millisecond)
	// 创建带超时上下文，避免测试阻塞。
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	// 订阅事件流。
	events, err := source.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	// 第一条事件应该是 connected。
	first := <-events
	if !first.Connected {
		t.Fatalf("expected first event to be connected, got: %+v", first)
	}
	// 后续应至少收到一条 disconnected 事件。
	for event := range events {
		if !event.Connected {
			return
		}
	}
	t.Fatal("expected a disconnected event after stream closed")
}
