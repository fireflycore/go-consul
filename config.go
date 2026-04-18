package consul

import "github.com/fireflycore/go-utils/tlsx"

// Config 定义 Consul 客户端初始化配置。
type Config struct {
	// Address 是 Consul HTTP API 地址，例如 127.0.0.1:8500。
	Address string `json:"address"`
	// Scheme 是访问协议，常见值为 http 或 https。
	Scheme string `json:"scheme"`
	// Datacenter 指定目标数据中心。
	Datacenter string `json:"datacenter"`
	// Token 是 ACL 访问令牌。
	Token string `json:"token"`

	// Tls 是可选 TLS 连接配置。
	Tls *tlsx.TLS `json:"tls"`
}
