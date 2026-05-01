package agent

import "strings"

// DefaultSidecarAgentConfig 返回一份可直接接入业务服务的默认 sidecar-agent 配置。
func DefaultSidecarAgentConfig(baseURL string) SidecarAgentConfig {
	// 先把外部传入的基础地址做去空白和去尾斜杠处理。
	cleanBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	// 当调用方没有提供基础地址时，回退到本机默认管理端口。
	if cleanBaseURL == "" {
		cleanBaseURL = DefaultAdminBaseURL
	}
	// 返回一份具备默认 watch 地址、请求超时和重连间隔的标准配置。
	return SidecarAgentConfig{
		// BaseURL 作为 register / drain / deregister 的基础地址。
		BaseURL: cleanBaseURL,
		// WatchURL 默认拼接本机 watch 路径。
		WatchURL: cleanBaseURL + DefaultWatchPath,
		// RequestTimeout 为管理接口调用提供默认超时。
		RequestTimeout: DefaultRequestTimeout,
		// ReconnectInterval 为 watch 断开后的重连提供默认节流。
		ReconnectInterval: DefaultReconnectInterval,
	}
}

// normalizeSidecarAgentConfig 把业务侧传入的 sidecar 配置补齐为可直接运行的完整配置。
func normalizeSidecarAgentConfig(config SidecarAgentConfig) SidecarAgentConfig {
	// 先基于传入 BaseURL 生成一份标准默认配置，便于后续回退。
	defaults := DefaultSidecarAgentConfig(config.BaseURL)
	// 如果调用方没有给 BaseURL，就直接回退到默认值。
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = defaults.BaseURL
	} else {
		// 否则把 BaseURL 做最小规范化，避免后续 URL 拼接出双斜杠。
		config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	}
	// 如果没有单独指定 WatchURL，就在规范化后的 BaseURL 上拼默认路径。
	if strings.TrimSpace(config.WatchURL) == "" {
		config.WatchURL = config.BaseURL + DefaultWatchPath
	} else {
		// 否则只做去空白处理，保留调用方显式给出的地址。
		config.WatchURL = strings.TrimSpace(config.WatchURL)
	}
	// 如果请求超时未显式配置，则回退到默认值。
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = defaults.RequestTimeout
	}
	// 如果重连间隔未显式配置，则回退到默认值。
	if config.ReconnectInterval <= 0 {
		config.ReconnectInterval = defaults.ReconnectInterval
	}
	// 返回补齐后的可运行配置。
	return config
}
