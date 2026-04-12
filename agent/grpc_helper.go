package agent

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc"
)

// GRPCDescriptorOptions 描述如何从 gRPC 服务描述直接构造 agent 注册信息。
type GRPCDescriptorOptions struct {
	// AppID 表示应用标识。
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
	// Protocol 表示业务协议。
	Protocol string
	// Version 表示业务版本号。
	Version string
	// ServiceOptions 表示业务服务当前的裸机接入配置。
	ServiceOptions *ServiceOptions
	// RawServices 表示业务已注册的 gRPC ServiceDesc 集合。
	RawServices []*grpc.ServiceDesc
}

// NewServiceRegistrationFromGRPC 使用 gRPC ServiceDesc 与 ServiceOptions 构造服务注册描述。
func NewServiceRegistrationFromGRPC(options GRPCDescriptorOptions) (ServiceRegistration, error) {
	// 服务名与端口是构造 sidecar-agent register 请求的最小前提。
	if strings.TrimSpace(options.ServiceName) == "" {
		return ServiceRegistration{}, errors.New("service name is required")
	}
	if options.Port <= 0 {
		return ServiceRegistration{}, errors.New("port is required")
	}
	// 创建业务服务节点描述，并从 ServiceOptions 中补齐默认值。
	node := &ServiceNode{
		Methods: make(map[string]bool),
		Meta: &ServiceMeta{
			AppId:   strings.TrimSpace(options.AppId),
			Version: strings.TrimSpace(options.Version),
			Env:     strings.TrimSpace(options.Env),
		},
	}
	if options.ServiceOptions != nil {
		// Bootstrap 会补齐 kernel、weight 等默认值。
		options.ServiceOptions.Bootstrap()
		node.Weight = options.ServiceOptions.Weight
		node.Kernel = options.ServiceOptions.Kernel
		node.Meta.InstanceId = strings.TrimSpace(options.ServiceOptions.InstanceId)
	}
	// 逐个解析 gRPC service desc，转换为统一 method 路径。
	for _, desc := range options.RawServices {
		if desc == nil {
			continue
		}
		for _, method := range desc.Methods {
			node.Methods[fmt.Sprintf("/%s/%s", desc.ServiceName, method.MethodName)] = true
		}
	}
	// 组装 sidecar-agent 服务注册描述，供后续运行时直接复用。
	return ServiceRegistration{
		AppId:       strings.TrimSpace(options.AppId),
		AppName:     strings.TrimSpace(options.AppName),
		ServiceName: strings.TrimSpace(options.ServiceName),
		Namespace:   strings.TrimSpace(options.Namespace),
		DNS:         strings.TrimSpace(options.DNS),
		Env:         strings.TrimSpace(options.Env),
		Port:        options.Port,
		Protocol:    strings.TrimSpace(options.Protocol),
		Version:     strings.TrimSpace(options.Version),
		Node:        node,
	}, nil
}

// NewServiceLifecycleFromGRPC 使用 gRPC ServiceDesc 与 ServiceOptions 直接创建业务生命周期桥接对象。
func NewServiceLifecycleFromGRPC(descriptorOptions GRPCDescriptorOptions, runtimeOptions LocalRuntimeOptions, lifecycleOptions LifecycleOptions) (*ServiceLifecycle, error) {
	// 先把 gRPC 描述转换成统一服务注册描述。
	descriptor, err := NewServiceRegistrationFromGRPC(descriptorOptions)
	if err != nil {
		return nil, err
	}
	// 再复用已有入口组装完整生命周期桥接对象。
	return NewServiceLifecycleFromServiceRegistration(descriptor, runtimeOptions, lifecycleOptions)
}
