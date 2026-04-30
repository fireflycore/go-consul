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
		// 把 provider 构造失败包装成更直接的 register replay 上下文错误。
		wrappedErr := fmt.Errorf("build register request failed: %w", err)
		// 在返回前把错误写入状态快照，便于业务侧直接查看最近失败原因。
		c.RecordError(wrappedErr)
		// 将包装后的错误返回给运行时，由运行时统一决定是否继续恢复流程。
		return wrappedErr
	}
	// 再通过 client 把注册请求发给本机 sidecar-agent。
	if err := c.client.Register(ctx, request); err != nil {
		// 把底层注册失败包装成带服务名与端口的重放错误，便于直接定位失败实例。
		replayErr := &RegisterReplayError{
			ServiceName: request.Name,
			ServicePort: request.Port,
			Err:         err,
		}
		// 写锁保护状态快照，避免和外部读取并发冲突。
		c.mu.Lock()
		// 记录一次 register replay 失败次数，供恢复链路排障使用。
		c.status.RegisterReplayFailureCount++
		// 即使失败也保留最近尝试重放的服务名，便于定位具体服务。
		c.status.LastServiceName = request.Name
		// 同步保留最近尝试重放的服务端口。
		c.status.LastServicePort = request.Port
		// 把包装后的失败错误写入统一状态快照。
		c.recordErrorLocked(replayErr, time.Now().UTC())
		// 完成本次失败状态写入后立即释放锁。
		c.mu.Unlock()
		// 将带上下文的失败错误返回给运行时。
		return replayErr
	}
	// 注册成功后把最新请求与状态写回控制器缓存。
	c.mu.Lock()
	// 函数结束前统一释放写锁，保证状态更新原子可见。
	defer c.mu.Unlock()
	// 记录本次连接恢复并重放成功的标准时间。
	now := time.Now().UTC()
	// 缓存最近一次成功注册请求，后续 drain / deregister 会复用它。
	c.last = &request
	// 重放成功后将连接状态标记为已连接。
	c.status.Connected = true
	// 同时标记最近一次 register 已成功完成。
	c.status.Registered = true
	// 回写最近一次成功接管的服务名。
	c.status.LastServiceName = request.Name
	// 回写最近一次成功接管的服务端口。
	c.status.LastServicePort = request.Port
	// 保存最近一次成功建立或恢复连接的时间。
	c.status.LastConnectedAt = formatStatusTime(now)
	// 累加成功的 register replay 次数，便于查看恢复频率。
	c.status.RegisterReplayCount++
	// 返回 nil 表示本轮恢复已经成功接管业务实例。
	return nil
}

// OnDisconnected 在本机 agent 连接断开时把控制器状态标记为未连接。
func (c *Controller) OnDisconnected() {
	// 更新状态时使用写锁，避免和读取状态并发冲突。
	c.mu.Lock()
	// 函数结束前释放写锁，保证断连状态写入完整可见。
	defer c.mu.Unlock()
	// 记录本次断连的统一时间基准。
	now := time.Now().UTC()
	// 连接断开后立即把 connected 标记为 false。
	c.status.Connected = false
	// 连接断开后不能再视为 register 已处于接管成功状态。
	c.status.Registered = false
	// 累加断连次数，便于观测恢复链路是否稳定。
	c.status.DisconnectCount++
	// 记录最近一次断连时间，方便与服务端日志对齐。
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
		AppId:         request.App.Id,
		AppInstanceId: request.App.InstanceId,

		ServiceName: request.Name,
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
		AppId:         request.App.Id,
		AppInstanceId: request.App.InstanceId,

		ServiceName: request.Name,
		ServicePort: request.ServerPort,
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
	// 写锁保护最近事件观测结果，避免读取方看到半更新状态。
	c.mu.Lock()
	// 在函数退出前释放锁。
	defer c.mu.Unlock()
	// 默认使用本地观测时间作为事件时间。
	observedAt := time.Now().UTC()
	if event.GeneratedAt != nil {
		// 如果服务端携带生成时间，则优先使用服务端时间对齐时序。
		observedAt = event.GeneratedAt.UTC()
	}
	// 记录最近一次收到的事件类型。
	c.status.LastEventType = event.Type
	// 记录最近一次收到的 SSE 事件 ID。
	c.status.LastEventId = event.EventId
	// 记录最近一次事件的观测时间。
	c.status.LastEventAt = formatStatusTime(observedAt)
	if event.Type == ConnectionEventTypeConnected || event.Connected {
		// 对 connected 事件补记最近一次连接成功时间。
		c.status.LastConnectedAt = formatStatusTime(observedAt)
	}
	if event.Type == ConnectionEventTypeDisconnected || !event.Connected {
		// 对 disconnected 事件补记最近一次断连时间。
		c.status.LastDisconnectedAt = formatStatusTime(observedAt)
	}
}

// RecordError 把最近一次运行错误写入状态快照。
func (c *Controller) RecordError(err error) {
	// 空错误不需要写入状态，直接返回即可。
	if err == nil {
		return
	}
	// 写锁保护最近错误快照，避免并发读写冲突。
	c.mu.Lock()
	// 在函数退出前统一释放锁。
	defer c.mu.Unlock()
	// 使用当前时间记录最近一次错误发生时刻。
	c.recordErrorLocked(err, time.Now().UTC())
}

// recordErrorLocked 在已持有写锁前提下写入最近错误快照。
func (c *Controller) recordErrorLocked(err error, observedAt time.Time) {
	// 先把错误归类成稳定的状态标签，方便文档和监控统一口径。
	c.status.LastErrorKind = classifyStatusError(err)
	// 再记录错误文本，便于在不解包错误的情况下直接排障。
	c.status.LastError = err.Error()
	// 最后记录错误发生时间，便于和连接事件时间对齐。
	c.status.LastErrorAt = formatStatusTime(observedAt)
}

// classifyStatusError 把运行时错误映射为稳定的状态分类。
func classifyStatusError(err error) string {
	switch {
	case err == nil:
		// 没有错误时返回空分类，表示当前无需展示错误标签。
		return ""
	case errors.Is(err, ErrWatchStreamClosed):
		// 已识别的 stream closed 错误单独归类，方便区分正常断流与协议异常。
		return "watch_stream_closed"
	}
	// 先识别非 200 响应这类连接前阶段错误。
	var statusErr *WatchHTTPStatusError
	if errors.As(err, &statusErr) {
		return "watch_http_status"
	}
	// 再识别结构化事件解析失败这类协议层错误。
	var parseErr *WatchEventParseError
	if errors.As(err, &parseErr) {
		return "watch_event_parse"
	}
	// register replay 失败需要保留独立分类，便于和 watch 错误区分。
	var replayErr *RegisterReplayError
	if errors.As(err, &replayErr) {
		return "register_replay"
	}
	// 其余错误统一归到通用运行时错误类别。
	return "runtime_error"
}

// formatStatusTime 统一把时间格式化成状态快照使用的 RFC3339Nano 字符串。
func formatStatusTime(value time.Time) string {
	// 零值时间统一返回空字符串，避免状态快照出现误导性时间戳。
	if value.IsZero() {
		return ""
	}
	// 统一转换为 UTC，确保不同节点日志对齐。
	return value.UTC().Format(time.RFC3339Nano)
}
