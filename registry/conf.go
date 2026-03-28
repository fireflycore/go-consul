package registry

import (
	"github.com/fireflycore/go-micro/constant"
	micro "github.com/fireflycore/go-micro/registry"
)

// ServiceConf 定义服务实例在 Consul 注册场景下的配置。
type ServiceConf struct {
	// 实例Id
	InstanceId string `json:"instance_id"`
	// 命名空间
	Namespace string `json:"namespace"`
	// 网卡
	Network *micro.Network `json:"network"`
	// 内核
	Kernel *micro.ServiceKernel `json:"kernel"`

	// 最大重试次数, 间隔时间是TTL*5
	MaxRetry uint32 `json:"max_retry"`
	// 心跳/租约 TTL（秒）, 最少是10s
	TTL uint32 `json:"ttl"`
	// 权重
	Weight int `json:"weight"`
}

// Bootstrap 补齐 namespace/ttl/maxRetry/network/kernel 等默认值，避免下游逻辑出现零值陷阱
func (sc *ServiceConf) Bootstrap() {
	// 命名空间为空时回退默认值。
	if sc.Namespace == "" {
		sc.Namespace = constant.DefaultNamespace
	}
	// 权重小于等于 0 时回退默认权重。
	if sc.Weight <= 0 {
		sc.Weight = 100
	}
	// 最大重试次数低于下限时提升到默认值。
	if sc.MaxRetry < constant.DefaultMaxRetry {
		sc.MaxRetry = constant.DefaultMaxRetry
	}
	// TTL 低于下限时提升到默认值。
	if sc.TTL < constant.DefaultTTL {
		sc.TTL = constant.DefaultTTL
	}

	// Kernel 为空时分配默认对象。
	if sc.Kernel == nil {
		sc.Kernel = &micro.ServiceKernel{}
	}
	// 统一补齐 Kernel 内部字段。
	sc.Kernel.Bootstrap()

	// Network 为空时分配默认对象。
	if sc.Network == nil {
		sc.Network = &micro.Network{}
	}
	// 统一补齐 Network 内部字段。
	sc.Network.Bootstrap()
}

// GatewayConf 定义网关相关配置。
type GatewayConf struct {
	// 网卡
	Network *micro.Network `json:"network"`
}

// Bootstrap 补齐网关网络配置默认值。
func (gc *GatewayConf) Bootstrap() {
	// 网关网络配置为空时分配默认对象。
	if gc.Network == nil {
		gc.Network = &micro.Network{}
	}
	// 统一补齐网关网络字段。
	gc.Network.Bootstrap()
}
