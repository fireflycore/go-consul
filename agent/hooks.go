package agent

import (
	"context"
	"errors"
)

// LifecycleHookOptions 描述如何把业务生命周期桥接对象接到宿主框架的启动钩子。
type LifecycleHookOptions struct {
	// Lifecycle 表示已经组装完成的业务生命周期桥接对象。
	Lifecycle *ServiceLifecycle
	// OnError 用于处理后台 watch 与 register 重放阶段的异步错误。
	OnError ErrorHandler
}

// LifecycleHooks 提供适合宿主框架 start/stop 钩子直接调用的适配层。
type LifecycleHooks struct {
	lifecycle *ServiceLifecycle
	onError   ErrorHandler
}

// NewLifecycleHooks 创建一个新的宿主钩子适配器。
func NewLifecycleHooks(options LifecycleHookOptions) (*LifecycleHooks, error) {
	if options.Lifecycle == nil {
		return nil, errors.New("service lifecycle is required")
	}
	return &LifecycleHooks{
		lifecycle: options.Lifecycle,
		onError:   options.OnError,
	}, nil
}

// OnStart 供宿主框架在启动阶段调用，内部会启动后台 watch 与自动重放循环。
func (h *LifecycleHooks) OnStart(ctx context.Context) error {
	errCh := h.lifecycle.Start(ctx)
	if h.onError != nil {
		go func() {
			for err := range errCh {
				if err != nil {
					h.onError(ctx, err)
				}
			}
		}()
	}
	return nil
}

// OnStop 供宿主框架在关闭阶段调用，内部统一执行 drain + deregister。
func (h *LifecycleHooks) OnStop(ctx context.Context) error {
	return h.lifecycle.Shutdown(ctx)
}

// Status 返回当前钩子适配层看到的最新运行状态。
func (h *LifecycleHooks) Status() Status {
	return h.lifecycle.Status()
}
