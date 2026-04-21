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
	// lifecycle 保存真正执行 start / shutdown 的业务生命周期对象。
	lifecycle *ServiceLifecycle
	// onError 保存宿主侧提供的异步错误处理回调。
	onError ErrorHandler
}

// NewLifecycleHooks 创建一个新的宿主钩子适配器。
func NewLifecycleHooks(options LifecycleHookOptions) (*LifecycleHooks, error) {
	// 宿主钩子适配层必须建立在完整的生命周期对象之上。
	if options.Lifecycle == nil {
		return nil, errors.New("service lifecycle is required")
	}
	// 返回一个可以直接挂到宿主框架启动/停止钩子的轻量适配层。
	return &LifecycleHooks{
		// 透传生命周期对象，内部仍由它负责真正的 agent 接管逻辑。
		lifecycle: options.Lifecycle,
		// 保存异步错误回调，供后台循环出错时向外转发。
		onError: options.OnError,
	}, nil
}

// OnStart 供宿主框架在启动阶段调用，内部会启动后台 watch 与自动重放循环。
func (h *LifecycleHooks) OnStart(ctx context.Context) error {
	// 启动业务生命周期，并拿到后台 watch / replay 循环输出的错误通道。
	errCh := h.lifecycle.Start(ctx)
	if h.onError != nil {
		// 如果宿主侧提供了异步错误处理器，就在后台持续消费错误通道。
		go func() {
			for err := range errCh {
				if err != nil {
					// 只转发非空错误，避免把空值当成有效事件。
					h.onError(ctx, err)
				}
			}
		}()
	}
	// 启动动作本身由 lifecycle.Start 完成，这里只返回同步启动结果。
	return nil
}

// OnStop 供宿主框架在关闭阶段调用，内部统一执行 drain + deregister。
func (h *LifecycleHooks) OnStop(ctx context.Context) error {
	// 停止阶段统一委托给生命周期对象执行 drain + deregister。
	return h.lifecycle.Shutdown(ctx)
}

// Status 返回当前钩子适配层看到的最新运行状态。
func (h *LifecycleHooks) Status() Status {
	// 直接复用生命周期对象维护的最新状态快照。
	return h.lifecycle.Status()
}
