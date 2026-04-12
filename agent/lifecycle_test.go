package agent

import (
	"context"
	"testing"
	"time"
)

// TestNewServiceLifecycleRequiresRuntime 验证生命周期桥接对象要求本地运行时不能为空。
func TestNewServiceLifecycleRequiresRuntime(t *testing.T) {
	// 当 runtime 为空时，应直接返回错误。
	_, err := NewServiceLifecycle(LifecycleOptions{})
	if err == nil {
		t.Fatal("expected nil runtime to fail")
	}
}

// TestServiceLifecycleShutdownDelegates 验证优雅关闭会先摘流再注销。
func TestServiceLifecycleShutdownDelegates(t *testing.T) {
	// 创建最小 fake client 与控制器。
	client := &fakeClient{}
	controller, err := NewController(client, fakeProvider{
		request: RegisterRequest{
			Name: "order",
			Port: 7070,
		},
	})
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	// 先建立一次成功注册，确保控制器缓存了最近一次请求。
	if err := controller.OnConnected(context.Background()); err != nil {
		t.Fatalf("on connected failed: %v", err)
	}
	// 组装最小本地运行时与生命周期桥接对象。
	runtime := &LocalRuntime{
		Controller: controller,
	}
	lifecycle, err := NewServiceLifecycle(LifecycleOptions{
		Runtime:     runtime,
		GracePeriod: "12s",
	})
	if err != nil {
		t.Fatalf("new service lifecycle failed: %v", err)
	}
	// 执行一次优雅关闭。
	if err := lifecycle.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	// 关闭流程应按顺序执行一次 drain 和一次 deregister。
	if got, want := len(client.drainCalls), 1; got != want {
		t.Fatalf("unexpected drain call count: got=%d want=%d", got, want)
	}
	if got, want := len(client.deregisterCalls), 1; got != want {
		t.Fatalf("unexpected deregister call count: got=%d want=%d", got, want)
	}
}

// TestServiceLifecycleStartReturnsErrorChannel 验证生命周期桥接对象会启动后台运行循环。
func TestServiceLifecycleStartReturnsErrorChannel(t *testing.T) {
	// 创建一个最小 fake provider 与默认参数本地运行时。
	runtime, err := NewLocalRuntime(fakeProvider{
		request: RegisterRequest{
			Name: "user",
			Port: 6060,
		},
	}, DefaultLocalRuntimeOptions(""))
	if err != nil {
		t.Fatalf("new local runtime failed: %v", err)
	}
	lifecycle, err := NewServiceLifecycle(LifecycleOptions{
		Runtime: runtime,
	})
	if err != nil {
		t.Fatalf("new service lifecycle failed: %v", err)
	}
	// 使用极短上下文启动生命周期，验证能正常返回错误通道而不 panic。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	errorsCh := lifecycle.Start(ctx)
	if errorsCh == nil {
		t.Fatal("expected non-nil error channel")
	}
}
