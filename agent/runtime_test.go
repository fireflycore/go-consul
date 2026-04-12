package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeEventSource 提供可控连接事件流，便于验证 Runner 行为。
type fakeEventSource struct {
	// events 保存测试输入的连接事件序列。
	events chan ConnectionEvent
	// err 用于模拟订阅阶段直接失败。
	err error
}

// Subscribe 返回测试侧注入的事件通道。
func (s *fakeEventSource) Subscribe(ctx context.Context) (<-chan ConnectionEvent, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.events, nil
}

// TestRunnerReplaysRegisterOnReconnect 验证连接恢复时会触发 register 重放。
func TestRunnerReplaysRegisterOnReconnect(t *testing.T) {
	// 创建最小 fake client 与控制器。
	client := &fakeClient{}
	controller, err := NewController(client, fakeProvider{
		request: RegisterRequest{
			Name: "auth",
			Port: 9090,
		},
	})
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	// 创建一个带缓冲的事件源，依次投递连接、断开、重连事件。
	source := &fakeEventSource{
		events: make(chan ConnectionEvent, 3),
	}
	runner, err := NewRunner(source, controller, nil)
	if err != nil {
		t.Fatalf("new runner failed: %v", err)
	}
	// 启动 Runner 后台循环。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()
	// 依次模拟首次连接、连接断开、连接恢复。
	source.events <- ConnectionEvent{Connected: true}
	source.events <- ConnectionEvent{Connected: false}
	source.events <- ConnectionEvent{Connected: true}
	time.Sleep(50 * time.Millisecond)
	cancel()
	runErr := <-done
	if runErr == nil || !errors.Is(runErr, context.Canceled) {
		t.Fatalf("unexpected runner error: %v", runErr)
	}
	if got, want := len(client.registerCalls), 2; got != want {
		t.Fatalf("unexpected register call count: got=%d want=%d", got, want)
	}
	// 最终状态应停留在 connected + registered。
	status := controller.Status()
	if !status.Connected || !status.Registered {
		t.Fatal("expected controller to be connected and registered after reconnect")
	}
}

// TestRunnerMarksDisconnectedWhenRegisterFails 验证 register 失败时会把状态回退为断连。
func TestRunnerMarksDisconnectedWhenRegisterFails(t *testing.T) {
	// 使用会失败的 client 构造控制器。
	controller, err := NewController(&failingClient{}, fakeProvider{
		request: RegisterRequest{
			Name: "payment",
			Port: 8080,
		},
	})
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	// 创建事件源，仅投递一次 connected 事件。
	source := &fakeEventSource{
		events: make(chan ConnectionEvent, 1),
	}
	// 记录错误回调次数，验证失败会被透传。
	errorCount := 0
	runner, err := NewRunner(source, controller, func(ctx context.Context, err error) {
		errorCount++
	})
	if err != nil {
		t.Fatalf("new runner failed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()
	// 连接建立后 register 会失败，Runner 应把状态回退到 disconnected。
	source.events <- ConnectionEvent{Connected: true}
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	status := controller.Status()
	if status.Connected || status.Registered {
		t.Fatal("expected controller to be disconnected after register failure")
	}
	if got, want := errorCount, 1; got != want {
		t.Fatalf("unexpected error callback count: got=%d want=%d", got, want)
	}
}

// failingClient 用于模拟本机 agent register 失败。
type failingClient struct{}

// Register 始终返回错误，模拟 sidecar-agent 不可接受注册。
func (c *failingClient) Register(ctx context.Context, request RegisterRequest) error {
	return errors.New("register failed")
}

// Drain 在该测试中不是重点，直接返回成功。
func (c *failingClient) Drain(ctx context.Context, request DrainRequest) error {
	return nil
}

// Deregister 在该测试中不是重点，直接返回成功。
func (c *failingClient) Deregister(ctx context.Context, request DeregisterRequest) error {
	return nil
}
