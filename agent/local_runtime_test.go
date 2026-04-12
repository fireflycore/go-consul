package agent

import (
	"context"
	"testing"
)

// TestDefaultLocalRuntimeOptions 验证默认参数会回退到本机 sidecar-agent 标准地址。
func TestDefaultLocalRuntimeOptions(t *testing.T) {
	// 当调用方不传基础地址时，应自动回退到默认值。
	options := DefaultLocalRuntimeOptions("")
	if got, want := options.BaseURL, DefaultAdminBaseURL; got != want {
		t.Fatalf("unexpected base url: got=%s want=%s", got, want)
	}
	if got, want := options.WatchURL, DefaultAdminBaseURL+DefaultWatchPath; got != want {
		t.Fatalf("unexpected watch url: got=%s want=%s", got, want)
	}
}

// TestNewLocalRuntimeNormalizesDefaults 验证本地运行时会自动补齐默认参数。
func TestNewLocalRuntimeNormalizesDefaults(t *testing.T) {
	// 使用最小 provider 构造本地运行时。
	runtime, err := NewLocalRuntime(fakeProvider{
		request: RegisterRequest{
			Name: "auth",
			Port: 9090,
		},
	}, LocalRuntimeOptions{})
	if err != nil {
		t.Fatalf("new local runtime failed: %v", err)
	}
	// 运行时应自动补齐默认 client 和 watch 地址。
	if got, want := runtime.Client.baseURL, DefaultAdminBaseURL; got != want {
		t.Fatalf("unexpected runtime base url: got=%s want=%s", got, want)
	}
	if got, want := runtime.Source.watchURL, DefaultAdminBaseURL+DefaultWatchPath; got != want {
		t.Fatalf("unexpected runtime watch url: got=%s want=%s", got, want)
	}
}

// TestLocalRuntimeDelegatesToController 验证本地运行时会把 drain 与 deregister 委托给控制器。
func TestLocalRuntimeDelegatesToController(t *testing.T) {
	// 创建最小 fake client 与控制器。
	client := &fakeClient{}
	controller, err := NewController(client, fakeProvider{
		request: RegisterRequest{
			Name: "payment",
			Port: 8080,
		},
	})
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	// 先完成一次成功连接，建立 last request。
	if err := controller.OnConnected(context.Background()); err != nil {
		t.Fatalf("on connected failed: %v", err)
	}
	// 手工组装一个最小 local runtime。
	runtime := &LocalRuntime{
		Controller: controller,
	}
	// 调用运行时级别的 drain 与 deregister。
	if err := runtime.Drain(context.Background(), "15s"); err != nil {
		t.Fatalf("runtime drain failed: %v", err)
	}
	if err := runtime.Deregister(context.Background()); err != nil {
		t.Fatalf("runtime deregister failed: %v", err)
	}
	// 两次委托调用都应真正命中 fake client。
	if got, want := len(client.drainCalls), 1; got != want {
		t.Fatalf("unexpected drain count: got=%d want=%d", got, want)
	}
	if got, want := len(client.deregisterCalls), 1; got != want {
		t.Fatalf("unexpected deregister count: got=%d want=%d", got, want)
	}
}
