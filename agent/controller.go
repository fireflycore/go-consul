package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// DescriptorProvider 抽象业务服务注册描述的构造能力。
type DescriptorProvider interface {
	// BuildRegisterRequest 返回当前业务服务完整注册描述。
	BuildRegisterRequest(ctx context.Context) (RegisterRequest, error)
}

// Client 抽象本机 sidecar-agent 的最小交互能力。
type Client interface {
	// Register 负责把当前服务注册到本机 sidecar-agent。
	Register(ctx context.Context, request RegisterRequest) error
	// Drain 负责把当前服务切换到摘流状态。
	Drain(ctx context.Context, request DrainRequest) error
	// Deregister 负责把当前服务从本机 sidecar-agent 注销。
	Deregister(ctx context.Context, request DeregisterRequest) error
}

// Controller 负责在业务服务侧缓存注册描述，并在连接恢复后重放 register。
type Controller struct {
	// client 保存与本机 sidecar-agent 交互的传输实现。
	client Client
	// provider 负责为当前业务服务构造标准注册描述。
	provider DescriptorProvider

	// mu 保护最近一次注册描述与状态快照。
	mu sync.RWMutex
	// last 保存最近一次成功构造的注册请求，供 drain 与 deregister 复用。
	last *RegisterRequest
	// status 保存当前控制器状态，供运行时观测使用。
	status Status
}

// NewController 创建一个新的业务侧 agent 联动控制器。
func NewController(client Client, provider DescriptorProvider) (*Controller, error) {
	// 对依赖做最小非空校验，避免控制器以不完整状态运行。
	switch {
	case client == nil:
		return nil, errors.New("agent client is required")
	case provider == nil:
		return nil, errors.New("descriptor provider is required")
	default:
		// 依赖齐备时返回一个可直接使用的控制器。
		return &Controller{
			client:   client,
			provider: provider,
		}, nil
	}
}

// OnConnected 在本机 agent 连接建立或恢复时重放 register。
func (c *Controller) OnConnected(ctx context.Context) error {
	// 先从 provider 构造一份最新注册描述。
	request, err := c.provider.BuildRegisterRequest(ctx)
	if err != nil {
		wrappedErr := fmt.Errorf("build register request failed: %w", err)
		c.RecordError(wrappedErr)
		return wrappedErr
	}
	// 再通过 client 把注册请求发给本机 sidecar-agent。
	if err := c.client.Register(ctx, request); err != nil {
		replayErr := &RegisterReplayError{
			ServiceName: request.Name,
			ServicePort: request.Port,
			Err:         err,
		}
		c.mu.Lock()
		c.status.RegisterReplayFailureCount++
		c.status.LastServiceName = request.Name
		c.status.LastServicePort = request.Port
		c.recordErrorLocked(replayErr, time.Now().UTC())
		c.mu.Unlock()
		return replayErr
	}
	// 注册成功后把最新请求与状态写回控制器缓存。
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now().UTC()
	c.last = &request
	c.status.Connected = true
	c.status.Registered = true
	c.status.LastServiceName = request.Name
	c.status.LastServicePort = request.Port
	c.status.LastConnectedAt = formatStatusTime(now)
	c.status.RegisterReplayCount++
	return nil
}

// OnDisconnected 在本机 agent 连接断开时把控制器状态标记为未连接。
func (c *Controller) OnDisconnected() {
	// 更新状态时使用写锁，避免和读取状态并发冲突。
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now().UTC()
	c.status.Connected = false
	c.status.Registered = false
	c.status.DisconnectCount++
	c.status.LastDisconnectedAt = formatStatusTime(now)
}

// Drain 使用最近一次成功注册的服务描述发起摘流。
func (c *Controller) Drain(ctx context.Context, gracePeriod string) error {
	// 先读取最近一次成功注册的服务描述。
	c.mu.RLock()
	request := c.last
	c.mu.RUnlock()
	// 若当前还没有完成过注册，则无法执行摘流。
	if request == nil {
		return errors.New("register request is not initialized")
	}
	// 复用最近一次注册时的服务名和端口构造摘流请求。
	return c.client.Drain(ctx, DrainRequest{
		Name:        request.Name,
		Port:        request.Port,
		GracePeriod: gracePeriod,
	})
}

// Deregister 使用最近一次成功注册的服务描述发起注销。
func (c *Controller) Deregister(ctx context.Context) error {
	// 先读取最近一次成功注册的服务描述。
	c.mu.RLock()
	request := c.last
	c.mu.RUnlock()
	// 若当前还没有完成过注册，则无法执行注销。
	if request == nil {
		return errors.New("register request is not initialized")
	}
	// 复用最近一次注册时的服务名和端口构造注销请求。
	return c.client.Deregister(ctx, DeregisterRequest{
		Name: request.Name,
		Port: request.Port,
	})
}

// Status 返回控制器的当前状态快照。
func (c *Controller) Status() Status {
	// 读取状态时使用读锁，避免暴露中间态。
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

// ObserveEvent 记录最近一次收到的 watch 事件，供业务侧调试恢复链路使用。
func (c *Controller) ObserveEvent(event ConnectionEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	observedAt := time.Now().UTC()
	if event.GeneratedAt != nil {
		observedAt = event.GeneratedAt.UTC()
	}
	c.status.LastEventType = event.Type
	c.status.LastEventId = event.EventId
	c.status.LastEventAt = formatStatusTime(observedAt)
	if event.Type == ConnectionEventTypeConnected || event.Connected {
		c.status.LastConnectedAt = formatStatusTime(observedAt)
	}
	if event.Type == ConnectionEventTypeDisconnected || !event.Connected {
		c.status.LastDisconnectedAt = formatStatusTime(observedAt)
	}
}

// RecordError 把最近一次运行错误写入状态快照。
func (c *Controller) RecordError(err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recordErrorLocked(err, time.Now().UTC())
}

func (c *Controller) recordErrorLocked(err error, observedAt time.Time) {
	c.status.LastErrorKind = classifyStatusError(err)
	c.status.LastError = err.Error()
	c.status.LastErrorAt = formatStatusTime(observedAt)
}

func classifyStatusError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrWatchStreamClosed):
		return "watch_stream_closed"
	}
	var statusErr *WatchHTTPStatusError
	if errors.As(err, &statusErr) {
		return "watch_http_status"
	}
	var parseErr *WatchEventParseError
	if errors.As(err, &parseErr) {
		return "watch_event_parse"
	}
	var replayErr *RegisterReplayError
	if errors.As(err, &replayErr) {
		return "register_replay"
	}
	return "runtime_error"
}

func formatStatusTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
