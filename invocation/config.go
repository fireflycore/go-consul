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

// Config 定义 Consul invocation 轻量实现配置。
type Config struct {
	// Namespace 是默认命名空间。
	Namespace string `json:"namespace"`
	// TargetOptions 表示通用 target 构造选项。
	TargetOptions microInvocation.TargetOptions `json:"target_options"`
	// PreferServiceDNS 控制是否优先走 service 级 DNS 目标。
	PreferServiceDNS bool `json:"prefer_service_dns"`
	// CacheTTL 控制健康实例列表缓存的有效期。
	CacheTTL time.Duration `json:"cache_ttl"`
}

// Bootstrap 补齐默认值。
func (c *Config) Bootstrap() {
	if c.Namespace == "" {
		c.Namespace = DefaultNamespace
	}
	if c.CacheTTL <= 0 {
		c.CacheTTL = DefaultCacheTTL
	}
	if c.TargetOptions.ClusterDomain == "" {
		c.TargetOptions.ClusterDomain = microInvocation.DefaultClusterDomain
	}
	if c.TargetOptions.ResolverScheme == "" {
		c.TargetOptions.ResolverScheme = microInvocation.DefaultResolverScheme
	}
}
