package agent

// KernelInfo 描述业务服务运行时信息。
type KernelInfo struct {
	// Language 表示业务服务使用的开发语言。
	Language string `json:"language"`
	// Version 表示业务服务运行时或框架版本。
	Version string `json:"version"`
}

// RegisterRequest 表示业务服务向本机 sidecar-agent 发起的注册请求。
type RegisterRequest struct {
	// AppID 表示应用标识。
	AppID string `json:"app_id"`
	// AppName 表示应用名称。
	AppName string `json:"app_name"`
	// Name 表示逻辑服务名。
	Name string `json:"name"`
	// Namespace 表示命名空间。
	Namespace string `json:"namespace"`
	// Port 表示业务服务监听端口。
	Port int `json:"port"`
	// DNS 表示业务服务统一域名。
	DNS string `json:"dns"`
	// Env 表示业务服务所属环境。
	Env string `json:"env"`
	// Weight 表示实例权重。
	Weight int `json:"weight"`
	// Protocol 表示服务协议。
	Protocol string `json:"protocol"`
	// Kernel 表示业务服务运行时信息。
	Kernel KernelInfo `json:"kernel"`
	// Methods 表示业务服务暴露的方法列表。
	Methods []string `json:"methods"`
	// Version 表示业务版本号。
	Version string `json:"version"`
}

// DrainRequest 表示业务服务向本机 sidecar-agent 发起的摘流请求。
type DrainRequest struct {
	// Name 表示逻辑服务名。
	Name string `json:"name"`
	// Port 表示业务服务监听端口。
	Port int `json:"port"`
	// GracePeriod 表示摘流宽限期。
	GracePeriod string `json:"grace_period"`
}

// DeregisterRequest 表示业务服务向本机 sidecar-agent 发起的注销请求。
type DeregisterRequest struct {
	// Name 表示逻辑服务名。
	Name string `json:"name"`
	// Port 表示业务服务监听端口。
	Port int `json:"port"`
}

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
