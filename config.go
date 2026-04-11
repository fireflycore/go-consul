package consul

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

	// TLS 配置 HTTPS 连接参数。
	TLS *TLS `json:"tls"`
}

// TLS 定义 Consul TLS 连接选项。
type TLS struct {
	// Enable 表示是否启用 TLS 配置。
	Enable bool `json:"enable"`
	// CaFile 指向 CA 证书文件路径。
	CaFile string `json:"ca_file"`
	// CertFile 指向客户端证书文件路径。
	CertFile string `json:"cert_file"`
	// KeyFile 指向客户端私钥文件路径。
	KeyFile string `json:"key_file"`
	// InsecureSkipVerify 控制是否跳过服务端证书校验。
	InsecureSkipVerify bool `json:"insecure_skip_verify"`
}
