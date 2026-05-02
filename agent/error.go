package agent

import "fmt"

const (
	// AgentRunStageLifecycle 表示 sidecar watch/replay 生命周期阶段。
	AgentRunStageLifecycle = "lifecycle"
	// AgentRunStageServe 表示业务服务阻塞运行阶段。
	AgentRunStageServe = "serve"
	// AgentRunStageShutdown 表示业务服务优雅关闭阶段。
	AgentRunStageShutdown = "shutdown"
	// AgentRunStageAgentShutdown 表示 agent 摘流与注销阶段。
	AgentRunStageAgentShutdown = "agent_shutdown"
)

// RegisterReplayError 表示业务侧在连接恢复后重放 register 时失败。
type RegisterReplayError struct {
	// ServiceName 表示当前重放失败的服务名。
	ServiceName string
	// ServicePort 表示当前重放失败的服务端口。
	ServicePort int
	// Err 表示底层 register 调用失败原因。
	Err error
}

// Error 返回带服务上下文的可读错误信息。
func (e *RegisterReplayError) Error() string {
	// 空错误对象统一返回空字符串，避免上层日志出现 `<nil>` 之类噪音。
	if e == nil {
		return ""
	}
	// 如果服务名和端口都具备，就输出最完整的服务实例上下文。
	if e.ServiceName != "" && e.ServicePort > 0 {
		return fmt.Sprintf("register replay failed: service=%s port=%d: %v", e.ServiceName, e.ServicePort, e.Err)
	}
	// 如果只有服务名，则至少保留服务级别上下文。
	if e.ServiceName != "" {
		return fmt.Sprintf("register replay failed: service=%s: %v", e.ServiceName, e.Err)
	}
	// 兜底情况下仅输出通用错误信息。
	return fmt.Sprintf("register replay failed: %v", e.Err)
}

// Unwrap 返回底层错误，便于调用方做 errors.Is / errors.As。
func (e *RegisterReplayError) Unwrap() error {
	// 空错误对象时没有可展开的底层错误。
	if e == nil {
		return nil
	}
	// 返回底层错误，支持进一步做错误分类。
	return e.Err
}

// LifecycleRunError 表示本地 agent watch/replay 后台循环运行失败。
type LifecycleRunError struct {
	// Err 表示底层运行失败原因。
	Err error
}

// Error 返回可读错误信息。
func (e *LifecycleRunError) Error() string {
	// 空错误对象统一返回空字符串。
	if e == nil {
		return ""
	}
	// 输出后台生命周期运行失败的标准错误文本。
	return fmt.Sprintf("agent lifecycle run failed: %v", e.Err)
}

// Unwrap 返回底层错误。
func (e *LifecycleRunError) Unwrap() error {
	// 空错误对象时没有可展开的底层错误。
	if e == nil {
		return nil
	}
	// 返回底层运行错误。
	return e.Err
}

// AgentRunError 表示单入口 Agent 在某个阶段失败。
type AgentRunError struct {
	// Stage 表示失败发生阶段。
	Stage string
	// Err 表示底层错误。
	Err error
}

// Error 返回带阶段信息的错误文本。
func (e *AgentRunError) Error() string {
	// 空错误对象统一返回空字符串。
	if e == nil {
		return ""
	}
	// 如果已经标明失败阶段，则把阶段信息一并输出。
	if e.Stage != "" {
		return fmt.Sprintf("agent %s failed: %v", e.Stage, e.Err)
	}
	// 否则输出通用的 Agent 运行失败信息。
	return fmt.Sprintf("agent run failed: %v", e.Err)
}

// Unwrap 返回底层错误。
func (e *AgentRunError) Unwrap() error {
	// 空错误对象时没有可展开的底层错误。
	if e == nil {
		return nil
	}
	// 返回底层错误，便于调用方做进一步匹配。
	return e.Err
}

// SidecarAPIError 表示 sidecar-agent 管理接口返回了非预期状态或失败结果。
type SidecarAPIError struct {
	Method     string
	Path       string
	StatusCode int
	Status     string
	Code       string
	Message    string
}

// Error 返回带方法、路径与 sidecar 稳定错误码的可读错误文本。
func (e *SidecarAPIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("sidecar api request failed: %s %s returned %d (%s): %s", e.Method, e.Path, e.StatusCode, e.Code, e.Message)
	}
	if e.Code != "" {
		return fmt.Sprintf("sidecar api request failed: %s %s returned %d (%s)", e.Method, e.Path, e.StatusCode, e.Code)
	}
	if e.Status != "" {
		return fmt.Sprintf("sidecar api request failed: %s %s returned %s", e.Method, e.Path, e.Status)
	}
	return fmt.Sprintf("sidecar api request failed: %s %s returned %d", e.Method, e.Path, e.StatusCode)
}
