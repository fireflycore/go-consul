package agent

import "fmt"

const (
	// ManagedServerStageLifecycle 表示 sidecar 生命周期桥接阶段。
	ManagedServerStageLifecycle = "lifecycle"
	// ManagedServerStageServe 表示业务服务阻塞运行阶段。
	ManagedServerStageServe = "serve"
	// ManagedServerStageShutdown 表示业务服务优雅关闭阶段。
	ManagedServerStageShutdown = "shutdown"
	// ManagedServerStageAgentShutdown 表示 agent 摘流与注销阶段。
	ManagedServerStageAgentShutdown = "agent_shutdown"
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
	if e == nil {
		return ""
	}
	if e.ServiceName != "" && e.ServicePort > 0 {
		return fmt.Sprintf("register replay failed: service=%s port=%d: %v", e.ServiceName, e.ServicePort, e.Err)
	}
	if e.ServiceName != "" {
		return fmt.Sprintf("register replay failed: service=%s: %v", e.ServiceName, e.Err)
	}
	return fmt.Sprintf("register replay failed: %v", e.Err)
}

// Unwrap 返回底层错误，便于调用方做 errors.Is / errors.As。
func (e *RegisterReplayError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// LifecycleRunError 表示本地 agent 生命周期后台循环运行失败。
type LifecycleRunError struct {
	// Err 表示底层运行失败原因。
	Err error
}

// Error 返回可读错误信息。
func (e *LifecycleRunError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("service lifecycle run failed: %v", e.Err)
}

// Unwrap 返回底层错误。
func (e *LifecycleRunError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ManagedServerRunError 表示托管服务在某个阶段失败。
type ManagedServerRunError struct {
	// Stage 表示失败发生阶段。
	Stage string
	// Err 表示底层错误。
	Err error
}

// Error 返回带阶段信息的错误文本。
func (e *ManagedServerRunError) Error() string {
	if e == nil {
		return ""
	}
	if e.Stage != "" {
		return fmt.Sprintf("managed server %s failed: %v", e.Stage, e.Err)
	}
	return fmt.Sprintf("managed server failed: %v", e.Err)
}

// Unwrap 返回底层错误。
func (e *ManagedServerRunError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
