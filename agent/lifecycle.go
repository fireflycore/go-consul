package agent

import (
	"context"
	"errors"
	"sync"
)

// LifecycleOptions 描述业务服务接入 agent 生命周期桥接时的最小参数。
type LifecycleOptions struct {
	// Runtime 表示已经组装完成的本地 agent 运行时。
	Runtime *LocalRuntime
	// GracePeriod 表示业务服务优雅下线时使用的默认摘流宽限期。
	GracePeriod string
}

// ServiceLifecycle 把本地 agent 运行时包装成更适合业务启动与退出阶段接入的对象。
type ServiceLifecycle struct {
	// runtime 保存底层本地 agent 运行时。
	runtime *LocalRuntime
	// gracePeriod 保存默认摘流宽限期。
	gracePeriod string
	// startOnce 保证后台运行循环只会被启动一次。
	startOnce sync.Once
	// errors 保存后台运行循环的异步错误通道。
	errors chan error
}

// NewServiceLifecycle 基于已组装好的本地运行时创建业务生命周期桥接对象。
func NewServiceLifecycle(options LifecycleOptions) (*ServiceLifecycle, error) {
	// 本地运行时为空时无法继续组装。
	if options.Runtime == nil {
		return nil, errors.New("local runtime is required")
	}
	// 返回一个可直接用于业务启动和退出的生命周期对象。
	return &ServiceLifecycle{
		runtime:     options.Runtime,
		gracePeriod: options.GracePeriod,
		errors:      make(chan error, 1),
	}, nil
}

// NewServiceLifecycleFromServiceRegistration 允许业务直接基于 go-micro 服务描述组装生命周期桥接对象。
func NewServiceLifecycleFromServiceRegistration(descriptor ServiceRegistration, runtimeOptions LocalRuntimeOptions, lifecycleOptions LifecycleOptions) (*ServiceLifecycle, error) {
	// 先用服务描述组装本地运行时。
	runtime, err := NewLocalRuntimeFromServiceRegistration(descriptor, runtimeOptions)
	if err != nil {
		return nil, err
	}
	// 再复用统一入口组装生命周期桥接对象。
	lifecycleOptions.Runtime = runtime
	return NewServiceLifecycle(lifecycleOptions)
}

// Start 在后台启动本地 watch 运行循环，并返回异步错误通道。
func (l *ServiceLifecycle) Start(ctx context.Context) <-chan error {
	// 保证后台运行循环只会启动一次，避免重复消费同一事件流。
	l.startOnce.Do(func() {
		go func() {
			// 运行结束后关闭错误通道，通知调用方不会再有新错误。
			defer close(l.errors)
			// 若运行循环异常退出，则把错误透传给调用方。
			if err := l.runtime.Run(ctx); err != nil && err != context.Canceled {
				l.errors <- err
			}
		}()
	})
	// 返回异步错误通道，供业务接入统一告警或日志系统。
	return l.errors
}

// Drain 使用默认宽限期对当前服务发起摘流。
func (l *ServiceLifecycle) Drain(ctx context.Context) error {
	// 直接委托给本地运行时执行摘流。
	return l.runtime.Drain(ctx, l.gracePeriod)
}

// Deregister 对当前服务发起注销。
func (l *ServiceLifecycle) Deregister(ctx context.Context) error {
	// 直接委托给本地运行时执行注销。
	return l.runtime.Deregister(ctx)
}

// Shutdown 先摘流再注销，适合业务服务优雅退出路径直接调用。
func (l *ServiceLifecycle) Shutdown(ctx context.Context) error {
	// 若配置了摘流宽限期，则优先执行摘流。
	if l.gracePeriod != "" {
		if err := l.runtime.Drain(ctx, l.gracePeriod); err != nil {
			return err
		}
	}
	// 摘流完成后再执行注销。
	return l.runtime.Deregister(ctx)
}

// Status 返回当前生命周期桥接对象的最新状态快照。
func (l *ServiceLifecycle) Status() Status {
	// 直接复用底层本地运行时的状态。
	return l.runtime.Status()
}
