package agent

import (
	"context"
	"strings"
	"time"
)

// LocalRuntimeOptions 描述业务服务接入本机 sidecar-agent 的最小参数。
type LocalRuntimeOptions struct {
	// BaseURL 表示本机 sidecar-agent 管理接口前缀。
	BaseURL string
	// WatchURL 表示本机 sidecar-agent 的长连接 watch 地址。
	WatchURL string
	// RequestTimeout 表示 register / drain / deregister 请求超时。
	RequestTimeout time.Duration
	// ReconnectInterval 表示 watch 断开后的重连间隔。
	ReconnectInterval time.Duration
	// OnError 用于统一处理 watch 与 register 重放过程中的错误。
	OnError ErrorHandler
}

// LocalRuntime 把本地 HTTP client、watch 事件源、控制器与运行器组装成一个整体。
type LocalRuntime struct {
	// Client 负责发起 register / drain / deregister 请求。
	Client *JSONHTTPClient
	// Source 负责把本地 `/watch` 连接转换成连接事件流。
	Source *WatchSource
	// Controller 负责缓存注册描述并在连接恢复时重放 register。
	Controller *Controller
	// Runner 负责驱动事件流与 register 重放过程。
	Runner *Runner
}

// NewLocalRuntime 使用最小参数组装一套可直接接入业务服务的本地运行时。
func NewLocalRuntime(provider DescriptorProvider, options LocalRuntimeOptions) (*LocalRuntime, error) {
	// 先补齐本地运行时默认值，降低业务接入成本。
	options = normalizeLocalRuntimeOptions(options)
	// 先创建本地 HTTP client，统一承接 register / drain / deregister 请求。
	client := NewJSONHTTPClient(options.BaseURL, options.RequestTimeout)
	// 再创建 watch 事件源，负责持续观察本机 agent 连接状态。
	source := NewWatchSource(options.WatchURL, options.ReconnectInterval)
	// 创建控制器，把业务注册描述与本机 client 绑定起来。
	controller, err := NewController(client, provider)
	if err != nil {
		return nil, err
	}
	// 创建运行器，把连接事件流转换成 register 重放动作。
	runner, err := NewRunner(source, controller, options.OnError)
	if err != nil {
		return nil, err
	}
	// 返回完整组装好的本地运行时。
	return &LocalRuntime{
		Client:     client,
		Source:     source,
		Controller: controller,
		Runner:     runner,
	}, nil
}

// Run 启动本地 watch 事件循环，并在连接恢复时自动重放 register。
func (r *LocalRuntime) Run(ctx context.Context) error {
	// 直接把运行控制权交给底层 Runner。
	return r.Runner.Run(ctx)
}

// Drain 通过控制器复用最近一次注册描述发起摘流。
func (r *LocalRuntime) Drain(ctx context.Context, gracePeriod string) error {
	// 直接调用控制器的统一摘流入口。
	return r.Controller.Drain(ctx, gracePeriod)
}

// Deregister 通过控制器复用最近一次注册描述发起注销。
func (r *LocalRuntime) Deregister(ctx context.Context) error {
	// 直接调用控制器的统一注销入口。
	return r.Controller.Deregister(ctx)
}

// Status 返回当前本地运行时的状态快照。
func (r *LocalRuntime) Status() Status {
	// 直接复用控制器内部状态，避免多处维护副本。
	return r.Controller.Status()
}

// normalizeLocalRuntimeOptions 为本地运行时参数补齐默认值。
func normalizeLocalRuntimeOptions(options LocalRuntimeOptions) LocalRuntimeOptions {
	// 基础地址为空时回退到默认值。
	if strings.TrimSpace(options.BaseURL) == "" {
		defaults := DefaultLocalRuntimeOptions("")
		options.BaseURL = defaults.BaseURL
	}
	// watch 地址为空时自动拼接默认 watch 路径。
	if strings.TrimSpace(options.WatchURL) == "" {
		options.WatchURL = strings.TrimRight(strings.TrimSpace(options.BaseURL), "/") + DefaultWatchPath
	}
	// 请求超时为空时回退到默认值。
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = DefaultRequestTimeout
	}
	// 重连间隔为空时回退到默认值。
	if options.ReconnectInterval <= 0 {
		options.ReconnectInterval = DefaultReconnectInterval
	}
	// 返回补齐默认值后的参数。
	return options
}
