package agent

import (
	"context"
	"testing"
	"time"
)

// TestNewManagedServerRequiresDependencies 验证托管器要求生命周期与服务入口都不能为空。
func TestNewManagedServerRequiresDependencies(t *testing.T) {
	// lifecycle 缺失时应直接失败。
	if _, err := NewManagedServer(ManagedServerOptions{}); err == nil {
		t.Fatal("expected missing lifecycle to fail")
	}
	// serve 缺失时同样应失败。
	lifecycle := newTestLifecycle(t)
	if _, err := NewManagedServer(ManagedServerOptions{
		Lifecycle: lifecycle,
	}); err == nil {
		t.Fatal("expected missing serve function to fail")
	}
}

// TestManagedServerRunTriggersLifecycleShutdown 验证业务服务退出时会统一执行生命周期关闭。
func TestManagedServerRunTriggersLifecycleShutdown(t *testing.T) {
	// 创建一份可工作的生命周期桥接对象。
	lifecycle := newTestLifecycle(t)
	// 记录业务服务 shutdown 是否被调用。
	shutdownCalled := false
	// 创建待测托管器。
	server, err := NewManagedServer(ManagedServerOptions{
		Lifecycle: lifecycle,
		Serve: func(ctx context.Context) error {
			// 业务服务很快正常退出，模拟启动失败或短任务退出场景。
			return nil
		},
		Shutdown: func(ctx context.Context) error {
			shutdownCalled = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("new managed server failed: %v", err)
	}
	// 执行 Run 后，应正常返回 nil。
	if err := server.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	// 业务服务 shutdown 应被执行。
	if !shutdownCalled {
		t.Fatal("expected shutdown to be called")
	}
	// 生命周期关闭后应产生一次 drain 与一次 deregister。
	client := lifecycle.runtime.Controller.client.(*fakeClient)
	if got, want := len(client.drainCalls), 1; got != want {
		t.Fatalf("unexpected drain call count: got=%d want=%d", got, want)
	}
	if got, want := len(client.deregisterCalls), 1; got != want {
		t.Fatalf("unexpected deregister call count: got=%d want=%d", got, want)
	}
}

// TestManagedServerRunReturnsLifecycleError 验证生命周期异常会透传给调用方。
func TestManagedServerRunReturnsLifecycleError(t *testing.T) {
	// 组装一个会立刻产生 lifecycle 错误的运行时。
	lifecycle := newTestLifecycle(t)
	lifecycle.runtime.Runner = &Runner{
		source:     failingEventSource{},
		controller: lifecycle.runtime.Controller,
	}
	server, err := NewManagedServer(ManagedServerOptions{
		Lifecycle: lifecycle,
		Serve: func(ctx context.Context) error {
			// 业务服务维持阻塞，等待 lifecycle 先报错。
			<-ctx.Done()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("new managed server failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := server.Run(ctx); err == nil {
		t.Fatal("expected lifecycle error to be returned")
	}
}

// failingEventSource 用于模拟生命周期事件源初始化失败。
type failingEventSource struct{}

// Subscribe 始终返回错误，模拟 watch 初始化失败。
func (failingEventSource) Subscribe(ctx context.Context) (<-chan ConnectionEvent, error) {
	return nil, context.DeadlineExceeded
}

// newTestLifecycle 创建一份可用于托管器测试的生命周期对象。
func newTestLifecycle(t *testing.T) *ServiceLifecycle {
	t.Helper()
	client := &fakeClient{}
	controller, err := NewController(client, fakeProvider{
		request: RegisterRequest{
			Name: "inventory",
			Port: 5050,
		},
	})
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	if err := controller.OnConnected(context.Background()); err != nil {
		t.Fatalf("on connected failed: %v", err)
	}
	runtime := &LocalRuntime{
		Controller: controller,
		Runner: &Runner{
			source: &fakeEventSource{
				events: make(chan ConnectionEvent),
			},
			controller: controller,
		},
	}
	lifecycle, err := NewServiceLifecycle(LifecycleOptions{
		Runtime:     runtime,
		GracePeriod: "15s",
	})
	if err != nil {
		t.Fatalf("new service lifecycle failed: %v", err)
	}
	return lifecycle
}
