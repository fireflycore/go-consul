package agent

import (
	"context"
	"errors"
	"sync"
)

// ServeFunc 抽象业务服务真正的阻塞运行入口。
type ServeFunc func(context.Context) error

// ShutdownFunc 抽象业务服务优雅关闭入口。
type ShutdownFunc func(context.Context) error

// Agent 是 go-consul/agent 对外唯一主入口，负责组装 watch/replay、注册摘流注销与可选的业务服务托管逻辑。
type Agent struct {
	// Node 保存当前业务服务构造出的固定 ServiceNode。
	Node *ServiceNode
	// Client 保存和 sidecar 管理接口交互的 API client。
	Client *ApiClient
	// Source 保存 sidecar watch 事件源。
	Source *WatchSource
	// Controller 保存基于固定节点的控制器。
	Controller *Controller
	// Runner 保存事件驱动运行器。
	Runner *Runner

	// gracePeriod 保存默认摘流宽限期。
	gracePeriod string
	// serve 保存业务服务自己的阻塞运行入口。
	serve ServeFunc
	// shutdown 保存业务服务自己的优雅关闭入口。
	shutdown ShutdownFunc

	// startOnce 保证后台 watch/replay 只启动一次。
	startOnce sync.Once
	// errors 保存后台运行过程中产生的异步错误。
	errors chan error
}

// New 使用 ServiceOptions 与 SidecarAgentConfig 组装一个可直接运行的 agent 主对象。
func New(serviceOptions *ServiceOptions, config SidecarAgentConfig) (*Agent, error) {
	// 没有业务服务配置时无法继续组装 Agent。
	if serviceOptions == nil {
		return nil, errors.New("service options are required")
	}

	// 先补齐 sidecar 配置默认值，降低业务接入成本。
	config = normalizeSidecarAgentConfig(config)
	// 再基于业务配置和 gRPC 描述构造固定 ServiceNode。
	node := NewServiceNode(serviceOptions, config.RawServices)
	// 理论上 node 不应为空，这里保留防御性检查。
	if node == nil {
		return nil, errors.New("service node is required")
	}

	// 创建底层 HTTP client，负责发送 JSON 管理请求。
	httpClient := NewHttpClient(config.BaseURL, config.RequestTimeout)
	// 创建更贴近业务语义的 API client。
	apiClient := NewApiClient(httpClient)
	// 创建 SSE watch 事件源。
	source := NewWatchSource(config.WatchURL, config.ReconnectInterval)
	// 创建固定持有 ServiceNode 的控制器。
	controller, err := NewController(apiClient, node)
	// 控制器创建失败时直接返回。
	if err != nil {
		return nil, err
	}
	// 创建事件驱动运行器，负责 watch/replay 主链。
	runner, err := NewRunner(source, controller, config.OnError)
	// 运行器创建失败时直接返回。
	if err != nil {
		return nil, err
	}

	// 返回已经组装完毕的单入口 Agent 主对象。
	return &Agent{
		// 暴露固定节点，方便调用方调试与观测。
		Node: node,
		// 暴露 sidecar 管理接口 client。
		Client: apiClient,
		// 暴露 watch 事件源，便于少量场景下调试。
		Source: source,
		// 暴露控制器，便于获取状态快照。
		Controller: controller,
		// 暴露运行器，便于补充运行期钩子。
		Runner: runner,
		// 保存默认摘流宽限期。
		gracePeriod: config.GracePeriod,
		// 保存业务服务运行函数。
		serve: config.Serve,
		// 保存业务服务关闭函数。
		shutdown: config.Shutdown,
		// 初始化异步错误通道。
		errors: make(chan error, 1),
	}, nil
}

// ConfigureRun 允许在创建 Agent 后补充业务 serve/shutdown 与运行期回调配置。
func (a *Agent) ConfigureRun(config SidecarAgentConfig) {
	// 空 Agent 直接忽略，避免调用方在 nil 上触发 panic。
	if a == nil {
		return
	}
	// 如果显式传入运行期错误处理器，则覆盖到 Runner。
	if config.OnError != nil && a.Runner != nil {
		a.Runner.onError = config.OnError
	}
	// 如果显式传入宽限期，则覆盖默认摘流宽限期。
	if config.GracePeriod != "" {
		a.gracePeriod = config.GracePeriod
	}
	// 如果显式传入业务运行函数，则覆盖已有配置。
	if config.Serve != nil {
		a.serve = config.Serve
	}
	// 如果显式传入业务关闭函数，则覆盖已有配置。
	if config.Shutdown != nil {
		a.shutdown = config.Shutdown
	}
}

// Start 在后台启动 watch/replay 主链，并返回异步错误通道。
func (a *Agent) Start(ctx context.Context) <-chan error {
	// 如果错误通道尚未初始化，则先初始化一份带缓冲通道。
	if a.errors == nil {
		a.errors = make(chan error, 1)
	}
	// 保证后台 watch/replay 协程只会启动一次。
	a.startOnce.Do(func() {
		go func() {
			// 后台协程退出时关闭错误通道，通知调用方不会再有新错误。
			defer close(a.errors)
			// 没有 Runner 时无法启动主链，这里主动返回一个生命周期错误。
			if a.Runner == nil {
				a.errors <- &LifecycleRunError{Err: errors.New("runner is required")}
				return
			}
			// 运行主链；上下文取消不视为异常，其余错误通过通道异步返回。
			if err := a.Runner.Run(ctx); err != nil && err != context.Canceled {
				a.errors <- &LifecycleRunError{Err: err}
			}
		}()
	})
	// 返回异步错误通道给业务侧消费。
	return a.errors
}

// Run 启动 agent 主链；如果配置了业务 serve/shutdown，也会统一托管业务服务生命周期。
func (a *Agent) Run(ctx context.Context) error {
	// 先启动 watch/replay 主链，并拿到异步错误通道。
	lifecycleErrCh := a.Start(ctx)

	// 默认没有业务 serve 时，不创建业务错误通道。
	var serveErrCh <-chan error
	// 如果业务侧提供了阻塞运行函数，则在后台启动它。
	if a.serve != nil {
		ch := make(chan error, 1)
		serveErrCh = ch
		go func() {
			// 把业务服务运行结果透传到统一主循环。
			ch <- a.serve(ctx)
		}()
	}

	// 持续等待上下文、生命周期错误或业务服务退出。
	for {
		select {
		case <-ctx.Done():
			// 外层上下文结束时，统一走收尾流程。
			return a.finishRun(nil)
		case err, ok := <-lifecycleErrCh:
			// 生命周期错误通道关闭后表示后台协程已结束。
			if !ok {
				// 如果没有业务服务要托管，则可以直接退出。
				if serveErrCh == nil {
					return nil
				}
				// 否则把生命周期通道置空，继续等待业务服务退出。
				lifecycleErrCh = nil
				continue
			}
			// 一旦收到生命周期错误，就包装成分阶段错误返回。
			if err != nil {
				return &AgentRunError{
					Stage: AgentRunStageLifecycle,
					Err:   err,
				}
			}
		case err := <-serveErrCh:
			// 业务服务退出后，统一走摘流、注销和关闭收尾。
			return a.finishRun(err)
		}
	}
}

// Drain 使用默认宽限期对当前服务发起摘流。
func (a *Agent) Drain(ctx context.Context) error {
	// 没有控制器时无法定位当前服务，直接返回错误。
	if a == nil || a.Controller == nil {
		return errors.New("controller is required")
	}
	// 统一委托控制器使用固定 ServiceNode 发起摘流。
	return a.Controller.Drain(ctx, a.gracePeriod)
}

// Deregister 对当前服务发起注销。
func (a *Agent) Deregister(ctx context.Context) error {
	// 没有控制器时无法定位当前服务，直接返回错误。
	if a == nil || a.Controller == nil {
		return errors.New("controller is required")
	}
	// 统一委托控制器使用固定 ServiceNode 发起注销。
	return a.Controller.Deregister(ctx)
}

// Shutdown 先摘流再注销，适合业务服务优雅退出路径直接调用。
func (a *Agent) Shutdown(ctx context.Context) error {
	// 没有控制器时无法执行摘流和注销，直接返回错误。
	if a == nil || a.Controller == nil {
		return errors.New("controller is required")
	}
	// 如果配置了宽限期，则先对当前服务执行摘流。
	if a.gracePeriod != "" {
		if err := a.Controller.Drain(ctx, a.gracePeriod); err != nil {
			return err
		}
	}
	// 摘流完成后，再执行最终注销。
	return a.Controller.Deregister(ctx)
}

// Status 返回当前 agent 的最新状态快照。
func (a *Agent) Status() Status {
	// 没有控制器时返回空状态，避免业务侧读取时 panic。
	if a == nil || a.Controller == nil {
		return Status{}
	}
	// 正常情况下直接复用控制器维护的最新状态快照。
	return a.Controller.Status()
}

// finishRun 统一处理 Run 的退出收尾逻辑。
func (a *Agent) finishRun(serveErr error) error {
	// 如果业务侧提供了关闭函数，则优先执行业务服务自己的关闭逻辑。
	if a.shutdown != nil {
		if err := a.shutdown(context.Background()); err != nil {
			return &AgentRunError{
				Stage: AgentRunStageShutdown,
				Err:   err,
			}
		}
	}
	// 无论业务服务是否报错，都执行 sidecar 摘流与注销收尾。
	if err := a.Shutdown(context.Background()); err != nil {
		return &AgentRunError{
			Stage: AgentRunStageAgentShutdown,
			Err:   err,
		}
	}
	// 如果业务服务运行阶段有错误，则在完成 sidecar 收尾后再向上返回。
	if serveErr != nil {
		return &AgentRunError{
			Stage: AgentRunStageServe,
			Err:   serveErr,
		}
	}
	// 所有收尾流程都成功时，返回 nil。
	return nil
}
