package agent

// Status 描述当前 agent 联动控制器的最新状态。
type Status struct {
	// Connected 表示当前是否与本机 sidecar-agent 保持连接。
	Connected bool
	// Registered 表示最近一次 register 是否成功完成。
	Registered bool
	// LastServiceName 表示最近一次成功注册的服务名。
	LastServiceName string
	// LastServicePort 表示最近一次成功注册的服务端口。
	LastServicePort int
	// LastEventType 表示最近一次收到的 watch 事件类型。
	LastEventType string
	// LastEventId 表示最近一次收到的 SSE 事件 ID。
	LastEventId string
	// LastEventAt 表示最近一次事件的观测时间。
	LastEventAt string
	// LastConnectedAt 表示最近一次成功建立或恢复连接的时间。
	LastConnectedAt string
	// LastDisconnectedAt 表示最近一次断连的时间。
	LastDisconnectedAt string
	// DisconnectCount 表示运行期间累计收到的断连次数。
	DisconnectCount int
	// RegisterReplayCount 表示累计成功重放 register 的次数。
	RegisterReplayCount int
	// RegisterReplayFailureCount 表示累计 register 重放失败次数。
	RegisterReplayFailureCount int
	// LastErrorKind 表示最近一次错误的分类。
	LastErrorKind string
	// LastError 表示最近一次错误文本。
	LastError string
	// LastErrorAt 表示最近一次错误发生时间。
	LastErrorAt string
}
