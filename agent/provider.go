package agent

import (
	"context"
	"errors"
	"strings"
)

// ServiceRegistration 描述如何把 go-micro 的服务信息映射成 sidecar-agent register 请求。
type ServiceRegistration struct {
	// AppId 表示应用标识；为空时优先回退到 Node.Meta.AppID。
	AppId string
	// AppName 表示应用名称；为空时回退到 ServiceName。
	AppName string
	// ServiceName 表示逻辑服务名。
	ServiceName string
	// Namespace 表示命名空间。
	Namespace string
	// DNS 表示统一服务域名。
	DNS string
	// Env 表示运行环境。
	Env string
	// Port 表示业务监听端口。
	Port int
	// Protocol 表示当前服务协议。
	Protocol string
	// Version 表示业务版本号。
	Version string
	// Weight 表示实例权重；为空时优先回退到 Node.Weight。
	Weight int
	// Node 表示业务服务当前的注册节点信息。
	Node *ServiceNode
}

// ServiceRegistrationProvider 负责把 go-micro 的服务节点信息转换成 agent 注册描述。
type ServiceRegistrationProvider struct {
	// descriptor 保存当前服务的静态注册描述。
	descriptor ServiceRegistration
}

// NewServiceRegistrationProvider 创建一个基于 go-micro 服务描述的 provider。
func NewServiceRegistrationProvider(descriptor ServiceRegistration) (*ServiceRegistrationProvider, error) {
	// 服务名与端口是 register 最小必要信息，缺失时直接返回错误。
	if strings.TrimSpace(descriptor.ServiceName) == "" {
		return nil, errors.New("service name is required")
	}
	if descriptor.Port <= 0 {
		return nil, errors.New("port is required")
	}
	// 构造 provider 时做一次固定描述校验，避免运行时反复失败。
	return &ServiceRegistrationProvider{
		descriptor: descriptor,
	}, nil
}

// BuildRegisterRequest 生成 sidecar-agent 所需的标准注册请求。
func (p *ServiceRegistrationProvider) BuildRegisterRequest(ctx context.Context) (RegisterRequest, error) {
	// 当前实现不依赖上下文，但保留参数以对齐统一 provider 接口。
	_ = ctx
	// 从静态描述中提取节点信息，便于后续回退默认值。
	node := p.descriptor.Node
	// 先决出最终 app_id，优先使用显式值。
	appID := strings.TrimSpace(p.descriptor.AppId)
	if appID == "" && node != nil && node.Meta != nil {
		appID = strings.TrimSpace(node.Meta.AppId)
	}
	// app_name 默认回退到逻辑服务名，减少业务接入样板代码。
	appName := strings.TrimSpace(p.descriptor.AppName)
	if appName == "" {
		appName = strings.TrimSpace(p.descriptor.ServiceName)
	}
	// 权重优先使用显式值，否则回退到节点权重，再回退到 100。
	weight := p.descriptor.Weight
	if weight <= 0 && node != nil {
		weight = node.Weight
	}
	if weight <= 0 {
		weight = 100
	}
	// kernel 与 methods 都允许为空，但优先从已有服务节点中复用。
	kernel := KernelInfo{}
	methods := []string{}
	if node != nil {
		if node.Kernel != nil {
			kernel.Language = strings.TrimSpace(node.Kernel.Language)
			kernel.Version = strings.TrimSpace(node.Kernel.Version)
		}
		for method := range node.Methods {
			methods = append(methods, method)
		}
	}
	// 返回一份 sidecar-agent 标准注册描述。
	return RegisterRequest{
		AppID:     appID,
		AppName:   appName,
		Name:      strings.TrimSpace(p.descriptor.ServiceName),
		Namespace: strings.TrimSpace(p.descriptor.Namespace),
		Port:      p.descriptor.Port,
		DNS:       strings.TrimSpace(p.descriptor.DNS),
		Env:       strings.TrimSpace(p.descriptor.Env),
		Weight:    weight,
		Protocol:  strings.TrimSpace(p.descriptor.Protocol),
		Kernel:    kernel,
		Methods:   methods,
		Version:   strings.TrimSpace(p.descriptor.Version),
	}, nil
}

// NewLocalRuntimeFromServiceRegistration 使用 go-micro 服务描述直接组装本地 agent 运行时。
func NewLocalRuntimeFromServiceRegistration(descriptor ServiceRegistration, options LocalRuntimeOptions) (*LocalRuntime, error) {
	// 先把 go-micro 服务描述转换成标准 provider。
	provider, err := NewServiceRegistrationProvider(descriptor)
	if err != nil {
		return nil, err
	}
	// 再用统一构造入口组装完整运行时。
	return NewLocalRuntime(provider, options)
}
