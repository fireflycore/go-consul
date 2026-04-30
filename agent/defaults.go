package agent

import (
	"strings"
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
