package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestNewLifecycleHooksRequiresLifecycle 验证宿主钩子适配层要求生命周期对象不能为空。
func TestNewLifecycleHooksRequiresLifecycle(t *testing.T) {
	// 不提供生命周期对象时应直接返回错误，避免生成不可用钩子。
	if _, err := NewLifecycleHooks(LifecycleHookOptions{}); err == nil {
		t.Fatal("expected missing lifecycle to fail")
	}
}

// TestLifecycleHooksOnStartAndOnStop 验证宿主钩子适配层会启动后台循环并在停止时执行摘流注销。
func TestLifecycleHooksOnStartAndOnStop(t *testing.T) {
	// 先构造一个完整的最小生命周期对象。
	lifecycle := newTestLifecycle(t)
	// 再基于生命周期对象创建宿主钩子适配层。
	hooks, err := NewLifecycleHooks(LifecycleHookOptions{
		Lifecycle: lifecycle,
	})
	if err != nil {
		t.Fatalf("new lifecycle hooks failed: %v", err)
	}

	// 创建可取消上下文，模拟宿主框架启动阶段上下文。
	ctx, cancel := context.WithCancel(context.Background())
	// 测试结束时确保后台上下文会被取消。
	defer cancel()
	// 启动钩子应成功拉起后台 watch / replay 循环。
	if err := hooks.OnStart(ctx); err != nil {
		t.Fatalf("on start failed: %v", err)
	}
	// 停止钩子应成功执行 drain + deregister。
	if err := hooks.OnStop(context.Background()); err != nil {
		t.Fatalf("on stop failed: %v", err)
	}

	// 取回 fake client，验证关闭阶段确实发生了摘流。
	client := lifecycle.runtime.Controller.client.(*fakeClient)
	if got, want := len(client.drainCalls), 1; got != want {
		t.Fatalf("unexpected drain count: got=%d want=%d", got, want)
	}
	// 同时验证关闭阶段确实发生了注销。
	if got, want := len(client.deregisterCalls), 1; got != want {
		t.Fatalf("unexpected deregister count: got=%d want=%d", got, want)
	}
}

// TestLifecycleHooksForwardsAsyncErrors 验证宿主钩子适配层会透传后台运行错误。
func TestLifecycleHooksForwardsAsyncErrors(t *testing.T) {
	// 先构造标准生命周期对象。
	lifecycle := newTestLifecycle(t)
	// 再把 Runner 替换成必然失败的事件源，制造后台异步错误。
	lifecycle.runtime.Runner = &Runner{
		source:     failingEventSource{},
		controller: lifecycle.runtime.Controller,
	}
	// 使用缓冲通道接收异步错误，避免测试竞争丢失回调。
	errCh := make(chan error, 1)
	// 创建带错误回调的宿主钩子适配层。
	hooks, err := NewLifecycleHooks(LifecycleHookOptions{
		Lifecycle: lifecycle,
		OnError: func(ctx context.Context, err error) {
			// 直接把异步错误转发到测试通道，供断言使用。
			errCh <- err
		},
	})
	if err != nil {
		t.Fatalf("new lifecycle hooks failed: %v", err)
	}

	// 为启动阶段设置一个短超时，避免后台失败时测试挂住。
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// 启动后应很快收到后台透传的生命周期运行错误。
	if err := hooks.OnStart(ctx); err != nil {
		t.Fatalf("on start failed: %v", err)
	}

	select {
	case asyncErr := <-errCh:
		// 透传到宿主侧的错误仍应保持为 LifecycleRunError。
		var lifecycleErr *LifecycleRunError
		if !errors.As(asyncErr, &lifecycleErr) {
			t.Fatalf("expected LifecycleRunError, got: %v", asyncErr)
		}
	case <-time.After(200 * time.Millisecond):
		// 超时仍未收到错误，则说明异步错误转发链路有缺口。
		t.Fatal("expected async lifecycle error")
	}
}
