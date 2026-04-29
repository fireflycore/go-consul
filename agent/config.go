package agent

import (
	"fmt"

	"github.com/fireflycore/go-micro/app"
	"github.com/fireflycore/go-micro/kernel"
	"github.com/fireflycore/go-micro/service"
)

type SidecarAgentConfig struct {
	// BaseURL 表示 sidecar-agent 基础 URL。
	BaseURL string `json:"base_url"`
	// GracePeriod 表示业务服务优雅下线时使用的默认摘流宽限期。
	GracePeriod string `json:"grace_period"`
}

type Options struct {
	// App 应用配置
	App *app.Config `json:"app"`
	// Kernel 内核配置
	Kernel *kernel.Config `json:"kernel"`
	// Service 服务配置
	Service *service.Config `json:"service"`

	// Protocol 表示业务协议, grpc / http。
	Protocol string `json:"protocol"`
	// ServerPort 服务端口
	ServerPort uint `json:"server_port"`
	// ManagePort 管理端口
	ManagedPort uint `json:"managed_port"`
}

func (o *Options) BuildDNS() string {
	// demo.default.svc.cluster.local:9090
	return fmt.Sprintf("%s.%s.%s.%s.%s:%d", o.Service.Name, o.Service.Namespace, o.Service.Type, o.Service.ClusterDomain, o.ServerPort)
}

// ServiceNode 描述业务服务在裸机场景下的最小注册节点信息。
type ServiceNodeInfo struct {
	*Options

	DNS     string `json:"dns"`
	RunDate string `json:"run_date"`

	// ProtoCount 表示业务服务子服务数量。
	ProtoCount uint `json:"proto_count"`
	// Methods 表示业务服务暴露的方法列表。
	Methods []string `json:"methods"`
}
