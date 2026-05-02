package agent

import (
	"fmt"
	"strings"
	"time"
)

// HealthCheckMode 表示业务服务覆盖给 sidecar 的主动探测方式。
type HealthCheckMode string

const (
	// HealthCheckModeAuto 表示根据协议自动选择探测方式。
	HealthCheckModeAuto HealthCheckMode = "auto"
	// HealthCheckModeTCP 表示使用 TCP 探测。
	HealthCheckModeTCP HealthCheckMode = "tcp"
	// HealthCheckModeHTTP 表示使用 HTTP GET 探测。
	HealthCheckModeHTTP HealthCheckMode = "http"
	// HealthCheckModeGRPC 表示使用 gRPC health 协议探测。
	HealthCheckModeGRPC HealthCheckMode = "grpc"
	// HealthCheckModeOff 表示关闭该服务的主动探测。
	HealthCheckModeOff HealthCheckMode = "off"
)

// HealthCheckConfig 描述业务服务在注册时覆盖给 sidecar 的主动探测参数。
type HealthCheckConfig struct {
	Enabled *bool           `json:"enabled,omitempty"`
	Mode    HealthCheckMode `json:"mode,omitempty"`
	Address string          `json:"address,omitempty"`
	Path    string          `json:"path,omitempty"`
	Service string          `json:"service,omitempty"`
}

// Validate 校验主动探测配置是否合法，保持和 sidecar-agent 正式模型一致。
func (c HealthCheckConfig) Validate() error {
	switch normalized := HealthCheckMode(strings.ToLower(strings.TrimSpace(string(c.Mode)))); normalized {
	case "", HealthCheckModeAuto, HealthCheckModeTCP, HealthCheckModeHTTP, HealthCheckModeGRPC, HealthCheckModeOff:
	default:
		return fmt.Errorf("health_check.mode is invalid: %s", c.Mode)
	}
	if trimmed := strings.TrimSpace(c.Address); trimmed != "" && !strings.Contains(trimmed, ":") {
		return fmt.Errorf("health_check.address is invalid: %s", c.Address)
	}
	if trimmed := strings.TrimSpace(c.Path); trimmed != "" && !strings.HasPrefix(trimmed, "/") {
		return fmt.Errorf("health_check.path is invalid: %s", c.Path)
	}
	return nil
}

// DrainRequest 表示业务服务向本机 sidecar-agent 发起的摘流请求。
type DrainRequest struct {
	// AppId 表示应用标识。
	AppId string `json:"app_id"`
	// AppInstanceId 表示应用实例标识。
	AppInstanceId string `json:"app_instance_id"`
	// GracePeriod 表示摘流宽限期。
	GracePeriod string `json:"grace_period"`
}

// DeregisterRequest 表示业务服务向本机 sidecar-agent 发起的注销请求。
type DeregisterRequest struct {
	// AppId 表示应用标识。
	AppId string `json:"app_id"`
	// AppInstanceId 表示应用实例标识。
	AppInstanceId string `json:"app_instance_id"`
}

// SidecarComponentStatus 描述 sidecar 单个运行时组件的最小状态。
type SidecarComponentStatus struct {
	Configured       bool       `json:"configured"`
	Started          bool       `json:"started"`
	Ready            bool       `json:"ready"`
	Detail           string     `json:"detail,omitempty"`
	LastTransitionAt *time.Time `json:"last_transition_at,omitempty"`
	LastError        string     `json:"last_error,omitempty"`
	LastErrorAt      *time.Time `json:"last_error_at,omitempty"`
}

// SidecarLocalServicesSummary 描述 sidecar 本机服务聚合状态。
type SidecarLocalServicesSummary struct {
	Total        int `json:"total"`
	Registered   int `json:"registered"`
	Serving      int `json:"serving"`
	Draining     int `json:"draining"`
	Deregistered int `json:"deregistered"`
}

// SidecarRuntimeSnapshot 描述 sidecar `debug/runtime` 和 `readyz` 里返回的主运行态视图。
type SidecarRuntimeSnapshot struct {
	Service              string                            `json:"service"`
	LifecycleState       string                            `json:"lifecycle_state"`
	Ready                bool                              `json:"ready"`
	Status               string                            `json:"status"`
	StartedAt            *time.Time                        `json:"started_at,omitempty"`
	Uptime               string                            `json:"uptime,omitempty"`
	Env                  string                            `json:"env"`
	Cluster              string                            `json:"cluster"`
	Zone                 string                            `json:"zone"`
	HostIP               string                            `json:"host_ip"`
	LocalServicesSummary SidecarLocalServicesSummary       `json:"local_services_summary"`
	NotReadyComponents   []string                          `json:"not_ready_components,omitempty"`
	Components           map[string]SidecarComponentStatus `json:"components"`
	Discovery            map[string]any                    `json:"discovery"`
	XDSSummary           map[string]any                    `json:"xds_summary"`
	EnvoySummary         map[string]any                    `json:"envoy_summary"`
	HealthCheckerSummary map[string]any                    `json:"health_checker_summary"`
}

// SidecarRecoverySnapshot 描述 sidecar `debug/recovery` 返回的恢复视图。
type SidecarRecoverySnapshot struct {
	Service              string                            `json:"service"`
	LifecycleState       string                            `json:"lifecycle_state"`
	Ready                bool                              `json:"ready"`
	Status               string                            `json:"status"`
	ShutdownRequested    bool                              `json:"shutdown_requested"`
	RunContextActive     bool                              `json:"run_context_active"`
	LocalServicesSummary SidecarLocalServicesSummary       `json:"local_services_summary"`
	FatalError           string                            `json:"fatal_error,omitempty"`
	FatalErrorAt         *time.Time                        `json:"fatal_error_at,omitempty"`
	NotReadyComponents   []string                          `json:"not_ready_components,omitempty"`
	Components           map[string]SidecarComponentStatus `json:"components"`
	Discovery            map[string]any                    `json:"discovery"`
	XDSSummary           map[string]any                    `json:"xds_summary"`
	EnvoySummary         map[string]any                    `json:"envoy_summary"`
	HealthCheckerSummary map[string]any                    `json:"health_checker_summary"`
	GeneratedAt          time.Time                         `json:"generated_at"`
}

// SidecarReadiness 描述 sidecar `readyz` 的返回结果。
type SidecarReadiness struct {
	StatusCode  int
	Success     bool
	Code        string
	Message     string
	GeneratedAt time.Time
	Snapshot    SidecarRuntimeSnapshot
}
