package agent

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Client 抽象本机 sidecar-agent 的最小交互能力。
type Client interface {
	// Register 负责把当前服务注册到本机 sidecar-agent。
	Register(ctx context.Context, request *ServiceNode) error
	// Drain 负责把当前服务切换到摘流状态。
	Drain(ctx context.Context, request DrainRequest) error
	// Deregister 负责把当前服务从本机 sidecar-agent 注销。
	Deregister(ctx context.Context, request DeregisterRequest) error
}

// Controller 负责在业务服务侧持有固定 ServiceNode，并在连接恢复后重放 register。
type Controller struct {
	// client 保存与本机 sidecar-agent 交互的传输实现。
	client Client
	// node 保存当前业务服务约定好的核心模型。
	node *ServiceNode

	// mu 保护状态快照。
	mu sync.RWMutex
	// status 保存当前控制器状态，供运行时观测使用。
	status Status
}

// NewController 创建一个新的业务侧 agent 联动控制器。
func NewController(client Client, node *ServiceNode) (*Controller, error) {
	switch {
	case client == nil:
		return nil, errors.New("agent client is required")
	case node == nil:
		return nil, errors.New("service node is required")
	case node.ServiceOptions == nil:
		return nil, errors.New("service options are required")
	case node.Service.Name == "":
		return nil, errors.New("service name is required")
	case node.ServerPort == 0:
		return nil, errors.New("service port is required")
	default:
		return &Controller{
			client: client,
			node:   node,
			status: Status{
				LastServiceName: node.Service.Name,
				LastServicePort: int(node.ServerPort),
			},
		}, nil
	}
}

// OnConnected 在本机 agent 连接建立或恢复时重放 register。
func (c *Controller) OnConnected(ctx context.Context) error {
	// 连接恢复后，先基于固定 ServiceNode 重放一次 register。
	if err := c.client.Register(ctx, c.node); err != nil {
		// 如果重放失败，则先包装成带服务上下文的稳定错误类型。
		replayErr := &RegisterReplayError{
			// 记录当前失败的逻辑服务名，便于日志和状态统一展示。
			ServiceName: c.node.Service.Name,
			// 记录当前失败的服务端口，便于唯一定位实例。
			ServicePort: int(c.node.ServerPort),
			// 保留底层注册失败原因，供上层继续解包。
			Err: err,
		}
		// 写锁保护失败状态更新，避免和状态读取方并发冲突。
		c.mu.Lock()
		// 累加 register replay 失败次数，便于观测恢复链路稳定性。
		c.status.RegisterReplayFailureCount++
		// 即使失败，也更新最近一次参与重放的服务名。
		c.status.LastServiceName = c.node.Service.Name
		// 即使失败，也更新最近一次参与重放的服务端口。
		c.status.LastServicePort = int(c.node.ServerPort)
		// 把失败错误和失败时间写入统一状态快照。
		c.recordErrorLocked(replayErr, time.Now().UTC())
		// 失败状态写完后立即释放锁。
		c.mu.Unlock()
		// 把带上下文的重放错误直接返回给上层。
		return replayErr
	}
	// register 重放成功后，开始更新连接成功状态。
	c.mu.Lock()
	// 在函数退出前释放锁，确保成功状态完整写入。
	defer c.mu.Unlock()
	// 统一使用当前 UTC 时间记录本次连接恢复时间。
	now := time.Now().UTC()
	// 连接恢复后标记当前已连上本机 sidecar-agent。
	c.status.Connected = true
	// register 成功后标记当前实例已重新接管成功。
	c.status.Registered = true
	// 更新最近一次成功注册的服务名。
	c.status.LastServiceName = c.node.Service.Name
	// 更新最近一次成功注册的服务端口。
	c.status.LastServicePort = int(c.node.ServerPort)
	// 记录最近一次连接恢复成功时间。
	c.status.LastConnectedAt = formatStatusTime(now)
	// 累加成功 register replay 次数。
	c.status.RegisterReplayCount++
	// 所有状态更新完成后返回成功。
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

// Drain 使用固定 ServiceNode 发起摘流。
func (c *Controller) Drain(ctx context.Context, gracePeriod string) error {
	return c.client.Drain(ctx, c.node.BuildDrainRequest(gracePeriod))
}

// Deregister 使用固定 ServiceNode 发起注销。
func (c *Controller) Deregister(ctx context.Context) error {
	return c.client.Deregister(ctx, c.node.BuildDeregisterRequest())
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
