package invocation

import (
	"time"

	microInvocation "github.com/fireflycore/go-micro/invocation"
)

const (
	// DefaultNamespace 是 Consul 轻量实现下的默认命名空间。
	DefaultNamespace = "default"
	// DefaultCacheTTL 是 Consul endpoint 缓存的默认有效期。
	DefaultCacheTTL = 5 * time.Second
)

// Conf 定义 Consul invocation 轻量实现配置。
type Conf struct {
	// Namespace 是默认命名空间。
	Namespace string `json:"namespace"`
	// DefaultPort 是默认 gRPC 端口。
	DefaultPort uint16 `json:"default_port"`
	// ClusterDomain 是 service DNS 使用的集群域。
	ClusterDomain string `json:"cluster_domain"`
	// ResolverScheme 是 gRPC target 使用的 resolver scheme。
	ResolverScheme string `json:"resolver_scheme"`
	// PreferServiceDNS 控制是否优先走 service 级 DNS 目标。
	PreferServiceDNS bool `json:"prefer_service_dns"`
	// CacheTTL 控制健康实例列表缓存的有效期。
	CacheTTL time.Duration `json:"cache_ttl"`
}

// Bootstrap 补齐默认值。
func (c *Conf) Bootstrap() {
	if c.Namespace == "" {
		c.Namespace = DefaultNamespace
	}
	if c.CacheTTL <= 0 {
		c.CacheTTL = DefaultCacheTTL
	}
	if c.ClusterDomain == "" {
		c.ClusterDomain = microInvocation.DefaultClusterDomain
	}
	if c.ResolverScheme == "" {
		c.ResolverScheme = microInvocation.DefaultResolverScheme
	}
}
