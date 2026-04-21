package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestNewLifecycleHooksRequiresLifecycle 验证宿主钩子适配层要求生命周期对象不能为空。
func TestNewLifecycleHooksRequiresLifecycle(t *testing.T) {
	if _, err := NewLifecycleHooks(LifecycleHookOptions{}); err == nil {
		t.Fatal("expected missing lifecycle to fail")
	}
}

// TestLifecycleHooksOnStartAndOnStop 验证宿主钩子适配层会启动后台循环并在停止时执行摘流注销。
func TestLifecycleHooksOnStartAndOnStop(t *testing.T) {
	lifecycle := newTestLifecycle(t)
	hooks, err := NewLifecycleHooks(LifecycleHookOptions{
		Lifecycle: lifecycle,
	})
	if err != nil {
		t.Fatalf("new lifecycle hooks failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := hooks.OnStart(ctx); err != nil {
		t.Fatalf("on start failed: %v", err)
	}
	if err := hooks.OnStop(context.Background()); err != nil {
		t.Fatalf("on stop failed: %v", err)
	}

	client := lifecycle.runtime.Controller.client.(*fakeClient)
	if got, want := len(client.drainCalls), 1; got != want {
		t.Fatalf("unexpected drain count: got=%d want=%d", got, want)
	}
	if got, want := len(client.deregisterCalls), 1; got != want {
		t.Fatalf("unexpected deregister count: got=%d want=%d", got, want)
	}
}

// TestLifecycleHooksForwardsAsyncErrors 验证宿主钩子适配层会透传后台运行错误。
func TestLifecycleHooksForwardsAsyncErrors(t *testing.T) {
	lifecycle := newTestLifecycle(t)
	lifecycle.runtime.Runner = &Runner{
		source:     failingEventSource{},
		controller: lifecycle.runtime.Controller,
	}
	errCh := make(chan error, 1)
	hooks, err := NewLifecycleHooks(LifecycleHookOptions{
		Lifecycle: lifecycle,
		OnError: func(ctx context.Context, err error) {
			errCh <- err
		},
	})
	if err != nil {
		t.Fatalf("new lifecycle hooks failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := hooks.OnStart(ctx); err != nil {
		t.Fatalf("on start failed: %v", err)
	}

	select {
	case asyncErr := <-errCh:
		var lifecycleErr *LifecycleRunError
		if !errors.As(asyncErr, &lifecycleErr) {
			t.Fatalf("expected LifecycleRunError, got: %v", asyncErr)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected async lifecycle error")
	}
}
