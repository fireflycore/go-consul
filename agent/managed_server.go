package agent

import (
	"context"
	"errors"
)

// ServeFunc 抽象业务服务真正的阻塞运行入口。
type ServeFunc func(context.Context) error

// ShutdownFunc 抽象业务服务优雅关闭入口。
type ShutdownFunc func(context.Context) error

// ManagedServerOptions 描述如何把业务服务运行逻辑与 agent 生命周期桥接到一起。
type ManagedServerOptions struct {
	// Lifecycle 表示 sidecar-agent 生命周期桥接对象。
	Lifecycle *ServiceLifecycle
	// Serve 表示业务服务真正的阻塞运行入口。
	Serve ServeFunc
	// Shutdown 表示业务服务优雅关闭入口；可为空。
	Shutdown ShutdownFunc
}

// ManagedServer 把业务服务运行逻辑与 sidecar-agent 生命周期收敛成一个统一入口。
type ManagedServer struct {
	// lifecycle 保存 sidecar-agent 生命周期桥接对象。
	lifecycle *ServiceLifecycle
	// serve 保存业务服务阻塞运行入口。
	serve ServeFunc
	// shutdown 保存业务服务优雅关闭入口。
	shutdown ShutdownFunc
}

// NewManagedServer 创建一个新的业务服务托管器。
func NewManagedServer(options ManagedServerOptions) (*ManagedServer, error) {
	// sidecar-agent 生命周期桥接对象为空时无法继续组装。
	if options.Lifecycle == nil {
		return nil, errors.New("service lifecycle is required")
	}
	// 业务服务运行入口为空时无法真正启动服务。
	if options.Serve == nil {
		return nil, errors.New("serve function is required")
	}
	// 返回一个可直接 Run 的统一托管器。
	return &ManagedServer{
		lifecycle: options.Lifecycle,
		serve:     options.Serve,
		shutdown:  options.Shutdown,
	}, nil
}

// Run 启动业务服务，并在退出时统一执行 agent 注销与业务优雅关闭。
func (s *ManagedServer) Run(ctx context.Context) error {
	// 先启动 sidecar-agent 生命周期桥接，确保后续连接恢复时能自动重放注册。
	lifecycleErrCh := s.lifecycle.Start(ctx)
	// 使用错误通道接收业务服务自身运行结果。
	serveErrCh := make(chan error, 1)
	go func() {
		// 把业务服务的运行结果透传给主循环统一处理。
		serveErrCh <- s.serve(ctx)
	}()
	for {
		select {
		case <-ctx.Done():
			// 外层上下文结束时，先优雅关闭业务服务，再摘流并注销。
			if s.shutdown != nil {
				if err := s.shutdown(context.Background()); err != nil {
					return err
				}
			}
			return s.lifecycle.Shutdown(context.Background())
		case err, ok := <-lifecycleErrCh:
			// 生命周期错误通道关闭后表示不会再有新错误。
			if !ok {
				lifecycleErrCh = nil
				continue
			}
			// 生命周期桥接异常退出时直接向上抛出。
			if err != nil {
				return err
			}
		case err := <-serveErrCh:
			// 业务服务主动退出时，仍然要执行统一注销流程。
			if s.shutdown != nil {
				if shutdownErr := s.shutdown(context.Background()); shutdownErr != nil {
					return shutdownErr
				}
			}
			if lifecycleErr := s.lifecycle.Shutdown(context.Background()); lifecycleErr != nil {
				return lifecycleErr
			}
			return err
		}
	}
}
