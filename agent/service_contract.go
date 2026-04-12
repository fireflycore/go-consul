package agent

import (
	"github.com/fireflycore/go-micro/constant"
)

// ServiceMeta 描述业务服务在裸机场景下的最小身份信息。
type ServiceMeta struct {
	AppId      string `json:"app_id"`
	InstanceId string `json:"instance_id"`
	Version    string `json:"version"`
	Env        string `json:"env"`
}

// ServiceKernel 描述业务服务运行时内核信息。
type ServiceKernel struct {
	Language string `json:"language"`
	Version  string `json:"version"`
}

// Bootstrap 补齐内核默认值。
func (k *ServiceKernel) Bootstrap() {
	k.Language = constant.KernelLanguage
	if k.Version == "" {
		k.Version = constant.DefaultVersion
	}
}

// ServiceOptions 描述业务服务接入本机 sidecar-agent 时的基础选项。
type ServiceOptions struct {
	InstanceId string         `json:"instance_id"`
	Namespace  string         `json:"namespace"`
	Kernel     *ServiceKernel `json:"kernel"`
	MaxRetry   uint32         `json:"max_retry"`
	TTL        uint32         `json:"ttl"`
	Weight     int            `json:"weight"`
}

// Bootstrap 补齐裸机服务接入时的默认值。
func (o *ServiceOptions) Bootstrap() {
	if o.Namespace == "" {
		o.Namespace = constant.DefaultNamespace
	}
	if o.Weight <= 0 {
		o.Weight = 100
	}
	if o.MaxRetry < constant.DefaultMaxRetry {
		o.MaxRetry = constant.DefaultMaxRetry
	}
	if o.TTL < constant.DefaultTTL {
		o.TTL = constant.DefaultTTL
	}
	if o.Kernel == nil {
		o.Kernel = &ServiceKernel{}
	}
	o.Kernel.Bootstrap()
}

// ServiceNode 描述业务服务在裸机场景下的最小注册节点信息。
type ServiceNode struct {
	ProtoCount int             `json:"proto_count"`
	Weight     int             `json:"weight"`
	RunDate    string          `json:"run_date"`
	Methods    map[string]bool `json:"methods"`
	Kernel     *ServiceKernel  `json:"kernel"`
	Meta       *ServiceMeta    `json:"meta"`
}
