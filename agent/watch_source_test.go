package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestWatchSourceParsesStructuredSSE 验证 watch 事件源会解析结构化 SSE 并区分 heartbeat。
func TestWatchSourceParsesStructuredSSE(t *testing.T) {
	connectedReady := true
	connectedAt := time.Now().UTC()
	heartbeatAt := connectedAt.Add(5 * time.Second)
	// 创建一个最小 SSE 服务端，先输出 connected 和 heartbeat，再主动关闭连接。
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
		connectedPayload, _ := json.Marshal(watchEventPayload{
			Event:          ConnectionEventTypeConnected,
			Message:        "watch stream established",
			Service:        "sidecar-agent",
			Status:         "ready",
			LifecycleState: "running",
			Ready:          &connectedReady,
			GeneratedAt:    &connectedAt,
		})
		heartbeatPayload, _ := json.Marshal(watchEventPayload{
			Event:          ConnectionEventTypeHeartbeat,
			Message:        "watch stream is alive",
			Service:        "sidecar-agent",
			Status:         "ready",
			LifecycleState: "running",
			Ready:          &connectedReady,
			GeneratedAt:    &heartbeatAt,
		})
		_, _ = writer.Write([]byte("id: 1\n"))
		_, _ = writer.Write([]byte("event: connected\n"))
		_, _ = writer.Write([]byte("data: "))
		_, _ = writer.Write(connectedPayload)
		_, _ = writer.Write([]byte("\n\n"))
		_, _ = writer.Write([]byte("id: 2\n"))
		_, _ = writer.Write([]byte("event: heartbeat\n"))
		_, _ = writer.Write([]byte("data: "))
		_, _ = writer.Write(heartbeatPayload)
		_, _ = writer.Write([]byte("\n\n"))
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
	if first.Type != ConnectionEventTypeConnected || !first.Connected {
		t.Fatalf("expected first event to be connected, got: %+v", first)
	}
	if first.EventId != "1" || first.Service != "sidecar-agent" || first.Status != "ready" || first.LifecycleState != "running" {
		t.Fatalf("unexpected connected payload: %+v", first)
	}
	if first.Ready == nil || !*first.Ready {
		t.Fatalf("expected ready flag in connected event: %+v", first)
	}
	// 第二条事件应为 heartbeat。
	second := <-events
	if second.Type != ConnectionEventTypeHeartbeat || second.Connected {
		t.Fatalf("expected heartbeat event, got: %+v", second)
	}
	if second.EventId != "2" {
		t.Fatalf("unexpected heartbeat event id: %+v", second)
	}
	// 后续应至少收到一条 disconnected 事件。
	for event := range events {
		if event.Type == ConnectionEventTypeDisconnected && !event.Connected {
			return
		}
	}
	t.Fatal("expected a disconnected event after stream closed")
}

// TestWatchSourceSupportsLegacyConnectedFrame 验证旧版 `event: connected + data: ok` 仍可兼容。
func TestWatchSourceSupportsLegacyConnectedFrame(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}
		_, _ = writer.Write([]byte("event: connected\n"))
		_, _ = writer.Write([]byte("data: ok\n\n"))
		flusher.Flush()
	}))
	defer server.Close()
	source := NewWatchSource(server.URL, 10*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	events, err := source.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	first := <-events
	if first.Type != ConnectionEventTypeConnected || !first.Connected {
		t.Fatalf("expected legacy frame to map to connected event, got: %+v", first)
	}
}

// TestWatchSourceReturnsHTTPStatusError 验证非 200 响应会返回可分类错误。
func TestWatchSourceReturnsHTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	source := NewWatchSource(server.URL, 10*time.Millisecond)
	err := source.watchOnce(context.Background(), make(chan ConnectionEvent, 1))
	var statusErr *WatchHTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected WatchHTTPStatusError, got: %v", err)
	}
	if statusErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status code: %+v", statusErr)
	}
}

// TestWatchSourceReturnsStreamClosedError 验证 EOF 会被分类为可识别的 stream closed 错误。
func TestWatchSourceReturnsStreamClosedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}
		_, _ = writer.Write([]byte("event: connected\n"))
		_, _ = writer.Write([]byte("data: ok\n\n"))
		flusher.Flush()
	}))
	defer server.Close()
	source := NewWatchSource(server.URL, 10*time.Millisecond)
	err := source.watchOnce(context.Background(), make(chan ConnectionEvent, 2))
	if !errors.Is(err, ErrWatchStreamClosed) {
		t.Fatalf("expected ErrWatchStreamClosed, got: %v", err)
	}
}

// TestWatchSourceSubscribeEmitsDisconnectedOnHTTPStatusError 验证订阅循环会把 HTTP 状态错误转换成断连事件。
func TestWatchSourceSubscribeEmitsDisconnectedOnHTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()
	source := NewWatchSource(server.URL, 10*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	events, err := source.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	event := <-events
	if event.Type != ConnectionEventTypeDisconnected || event.Connected {
		t.Fatalf("expected disconnected event, got: %+v", event)
	}
	var statusErr *WatchHTTPStatusError
	if !errors.As(event.Err, &statusErr) {
		t.Fatalf("expected WatchHTTPStatusError in event error, got: %v", event.Err)
	}
}

// TestWatchSourceSubscribeReconnectsAfterBackoff 验证订阅循环会在退避后发起下一次重连。
func TestWatchSourceSubscribeReconnectsAfterBackoff(t *testing.T) {
	// 统计服务端被访问次数，用于确认客户端确实发起了重连。
	var callCount atomic.Int32
	// 记录每次请求到达时间，用于断言是否经过了退避间隔。
	requestTimes := make(chan time.Time, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// 为每次 watch 请求分配顺序号。
		current := callCount.Add(1)
		// 把请求到达时间写入测试通道，供后续比较退避间隔。
		requestTimes <- time.Now()
		if current == 1 {
			// 第一次请求主动返回 503，制造一次需要退避重连的失败。
			http.Error(writer, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		// 第二次请求开始恢复正常 SSE 输出，模拟重连成功。
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}
		// 输出最小 connected 事件，驱动客户端产生恢复成功事件。
		_, _ = writer.Write([]byte("event: connected\n"))
		_, _ = writer.Write([]byte("data: ok\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	// 显式指定一个较容易观测的退避间隔。
	reconnectInterval := 40 * time.Millisecond
	// 创建待测 watch 事件源。
	source := NewWatchSource(server.URL, reconnectInterval)
	// 给整个订阅流程一个总超时，避免测试阻塞。
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	// 开始订阅事件流。
	events, err := source.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// 首条事件应来自第一次失败后的 disconnected 通知。
	first := <-events
	if first.Type != ConnectionEventTypeDisconnected {
		t.Fatalf("expected first event to be disconnected, got: %+v", first)
	}
	// 第二条事件应来自退避后重连成功的 connected 通知。
	second := <-events
	if second.Type != ConnectionEventTypeConnected {
		t.Fatalf("expected second event to be connected, got: %+v", second)
	}

	// 读取第一次失败请求的到达时间。
	firstRequestAt := <-requestTimes
	// 读取第二次重连请求的到达时间。
	secondRequestAt := <-requestTimes
	// 断言两次请求之间至少经历了设定的退避时间。
	if secondRequestAt.Sub(firstRequestAt) < reconnectInterval {
		t.Fatalf("expected reconnect backoff >= %s, got %s", reconnectInterval, secondRequestAt.Sub(firstRequestAt))
	}
}

// TestWatchSourceSubscribeEmitsRepeatedDisconnectedEvents 验证重复失败时会持续发出断连事件。
func TestWatchSourceSubscribeEmitsRepeatedDisconnectedEvents(t *testing.T) {
	// 统计失败请求次数，用于确认客户端不是只尝试一次。
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// 每次请求都计数，便于最后断言重复失败期间确实发生了多次重试。
		callCount.Add(1)
		// 持续返回 502，制造重复失败场景。
		http.Error(writer, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	// 使用较短重连间隔，加快重复失败场景下的测试收敛速度。
	source := NewWatchSource(server.URL, 10*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	// 开始订阅事件流。
	events, err := source.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// 统计收到的 disconnected 事件次数。
	disconnectedCount := 0
	// 设置一个局部超时，确保测试能及时失败而不是无限等待。
	deadline := time.After(120 * time.Millisecond)
	for disconnectedCount < 2 {
		select {
		case event := <-events:
			// 连续失败期间收到的事件都应为 disconnected。
			if event.Type != ConnectionEventTypeDisconnected {
				t.Fatalf("expected disconnected event, got: %+v", event)
			}
			// 每收到一次断连事件就累加一次计数。
			disconnectedCount++
		case <-deadline:
			// 若迟迟收不到第二次断连事件，说明重复失败重试链路存在缺口。
			t.Fatalf("expected repeated disconnected events, got=%d", disconnectedCount)
		}
	}
	// 服务端被访问次数至少应达到两次，证明客户端确实执行了重复重连尝试。
	if got := callCount.Load(); got < 2 {
		t.Fatalf("expected at least two subscribe attempts, got=%d", got)
	}
}

// TestEmitWatchEventIgnoresEmptyFrame 验证纯空帧不会输出任何事件。
func TestEmitWatchEventIgnoresEmptyFrame(t *testing.T) {
	ctx := context.Background()
	events := make(chan ConnectionEvent, 1)
	if err := emitWatchEvent(ctx, events, "", "", nil); err != nil {
		t.Fatalf("emitWatchEvent failed: %v", err)
	}
	select {
	case event := <-events:
		t.Fatalf("expected no event, got: %+v", event)
	default:
	}
}

// TestEmitWatchEventReturnsParseErrorForMalformedStructuredPayload 验证结构化坏帧会返回可分类错误。
func TestEmitWatchEventReturnsParseErrorForMalformedStructuredPayload(t *testing.T) {
	ctx := context.Background()
	events := make(chan ConnectionEvent, 1)
	err := emitWatchEvent(ctx, events, "connected", "42", []string{`{"event":"connected"`})
	var parseErr *WatchEventParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected WatchEventParseError, got: %v", err)
	}
	if parseErr.EventType != "connected" || parseErr.EventId != "42" {
		t.Fatalf("unexpected parse error payload: %+v", parseErr)
	}
}
