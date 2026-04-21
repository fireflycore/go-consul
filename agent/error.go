package agent

import "fmt"

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
