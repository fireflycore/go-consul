package agent

import (
	"strings"
	"time"
)

const (
	// DefaultAdminBaseURL 表示业务服务默认访问的本机 sidecar-agent 管理地址。
	DefaultAdminBaseURL = "http://127.0.0.1:15010"
	// DefaultWatchPath 表示 sidecar-agent 提供的默认 watch 路径。
	DefaultWatchPath = "/watch"
	// DefaultRequestTimeout 表示 register、drain、deregister 的默认请求超时。
	DefaultRequestTimeout = 3 * time.Second
	// DefaultReconnectInterval 表示 watch 断开后的默认重连间隔。
	DefaultReconnectInterval = time.Second
)

// DefaultLocalRuntimeOptions 返回一份可直接接入业务服务的默认本地运行时参数。
func DefaultLocalRuntimeOptions(baseURL string) LocalRuntimeOptions {
	// 当调用方未显式提供地址时，优先回退到本机默认管理地址。
	cleanBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if cleanBaseURL == "" {
		cleanBaseURL = DefaultAdminBaseURL
	}
	// 组装默认参数，供大多数业务服务直接复用。
	return LocalRuntimeOptions{
		BaseURL:           cleanBaseURL,
		WatchURL:          cleanBaseURL + DefaultWatchPath,
		RequestTimeout:    DefaultRequestTimeout,
		ReconnectInterval: DefaultReconnectInterval,
	}
}
