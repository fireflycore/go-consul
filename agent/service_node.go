package agent

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fireflycore/go-micro/app"
	"github.com/fireflycore/go-micro/kernel"
	"github.com/fireflycore/go-micro/service"
)

// SidecarAgentConfig 描述业务服务接入本机 sidecar-agent 时需要的运行配置。
type SidecarAgentConfig struct {
	// BaseURL 表示 sidecar-agent 基础 URL。
	BaseURL string `json:"base_url"`
	// WatchURL 表示 sidecar-agent 的 SSE watch 地址；为空时由 BaseURL 自动推导。
	WatchURL string `json:"watch_url"`
	// GracePeriod 表示业务服务优雅下线时使用的默认摘流宽限期。
	GracePeriod string `json:"grace_period"`
	// RequestTimeout 表示 register、drain、deregister 的请求超时。
	RequestTimeout time.Duration `json:"request_timeout"`
	// ReconnectInterval 表示 watch 断开后的重连间隔。
	ReconnectInterval time.Duration `json:"reconnect_interval"`
	// GatewayManifestPath 表示业务服务随构建产物携带的 gateway manifest 路径。
	GatewayManifestPath string `json:"gateway_manifest_path"`
	// OnError 用于统一处理 watch 与 register 重放过程中的异步错误。
	OnError ErrorHandler `json:"-"`
	// Serve 表示业务服务真正的阻塞运行入口；为空时仅运行 agent watch/replay 主链。
	Serve ServeFunc `json:"-"`
	// Shutdown 表示业务服务优雅关闭入口；可为空。
	Shutdown ShutdownFunc `json:"-"`
}

// ServiceOptions 描述业务服务自身的原始输入配置。
type ServiceOptions struct {
	// App 应用配置
	App app.Config `json:"app"`
	// Kernel 内核配置
	Kernel kernel.Config `json:"kernel"`
	// Service 服务配置
	Service service.Config `json:"service"`

	// Protocol 表示业务协议, grpc / http。
	Protocol string `json:"protocol"`
	// ServerPort 服务端口
	ServerPort uint `json:"server_port"`
	// ManagedPort 管理端口
	ManagedPort uint `json:"managed_port"`
}

// BuildDNS 基于当前服务配置拼接 sidecar 注册时使用的统一 DNS 地址。
func (o *ServiceOptions) BuildDNS() string {
	// sidecar-agent 会基于纯主机名再单独拼接 authority 端口，因此这里不能把端口写进 DNS。
	return fmt.Sprintf("%s.%s.%s.%s", o.Service.Name, o.Service.Namespace, o.Service.Type, o.Service.ClusterDomain)
}

// ServiceNode 描述业务服务在裸机场景下的最小注册节点信息。
type ServiceNode struct {
	*ServiceOptions

	DNS     string `json:"dns"`
	RunDate string `json:"run_date"`

	// ProtoCount 表示业务服务子服务数量。
	ProtoCount uint `json:"proto_count"`
	// Methods 表示业务服务暴露的方法列表。
	Methods []string `json:"methods"`
	// DescriptorRef 表示 api-gateway 加载 protobuf descriptor set 的 HTTP/HTTPS 地址。
	DescriptorRef string `json:"descriptor_ref,omitempty"`
	// HTTPRoutes 表示允许 HTTP 入口访问的 route，来源只能是 gateway manifest routes[]。
	HTTPRoutes []HTTPRoute `json:"http_routes,omitempty"`
}

// NewServiceNode 基于 ServiceOptions 和 gateway manifest 构造固定的业务服务节点描述。
func NewServiceNode(options *ServiceOptions, manifest *GatewayManifest) *ServiceNode {
	// 没有业务服务配置时无法构造节点，直接返回 nil。
	if options == nil {
		return nil
	}

	// 先复制一份 ServiceOptions，避免修改调用方传入对象。
	cloned := *options
	// 防止透传 app.Secret 到 sidecar-agent。
	cloned.App.Secret = ""

	// manifest 为空时保留零值能力，后续 Validate 会返回明确错误。
	var descriptorRef string
	var protoCount uint
	var methods []string
	var routes []HTTPRoute
	if manifest != nil {
		descriptorRef = manifest.DescriptorRef
		protoCount = manifest.ServiceCount()
		methods = manifest.MethodPaths()
		routes = manifest.HTTPRoutesCopy()
	}

	// 基于复制后的配置和 manifest 组装固定的 ServiceNode。
	node := &ServiceNode{
		// 持有一份去敏后的业务配置副本。
		ServiceOptions: &cloned,
		// 基于服务配置直接计算 sidecar 注册使用的统一 DNS。
		DNS: cloned.BuildDNS(),
		// 记录当前节点构造时间，便于 sidecar 侧观测。
		RunDate: time.Now().Format(time.RFC3339),
		// 记录 manifest 中声明的 gRPC service 数量。
		ProtoCount: protoCount,
		// 记录 manifest 中声明的完整方法路径集合。
		Methods: methods,
		// 透传 descriptor_ref；go-consul 只校验和上报，是否拉取 descriptor set 由 api-gateway 按 route 需求决定。
		DescriptorRef: descriptorRef,
		// 透传 HTTP route，HTTP 入口能力只能来自 manifest routes[]。
		HTTPRoutes: routes,
	}

	// 返回构造完成的固定服务节点描述。
	return node
}

// Validate 校验当前业务节点是否满足 sidecar-agent 的最小注册契约。
func (n *ServiceNode) Validate() error {
	// 当前节点与基础配置都不能为空。
	if n == nil {
		return errors.New("service node is required")
	}
	if n.ServiceOptions == nil {
		return errors.New("service options are required")
	}
	required := map[string]string{
		"app.id":            n.App.Id,
		"app.instance_id":   n.App.InstanceId,
		"app.name":          n.App.Name,
		"app.env":           n.App.Env,
		"app.version":       n.App.Version,
		"kernel.language":   n.Kernel.Language,
		"kernel.version":    n.Kernel.Version,
		"service.name":      n.Service.Name,
		"service.namespace": n.Service.Namespace,
		"protocol":          n.Protocol,
		"dns":               n.DNS,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", field)
		}
	}
	if n.ServerPort == 0 || n.ServerPort > 65535 {
		return fmt.Errorf("server_port is invalid: %d", n.ServerPort)
	}
	if n.Service.Weight == 0 {
		return errors.New("service.weight must be greater than zero")
	}
	if len(n.Methods) == 0 {
		return errors.New("methods must not be empty")
	}
	for _, method := range n.Methods {
		if !strings.HasPrefix(strings.TrimSpace(method), "/") {
			return fmt.Errorf("method is invalid: %s", method)
		}
	}
	methodSet := make(map[string]struct{}, len(n.Methods))
	for _, method := range n.Methods {
		methodSet[strings.TrimSpace(method)] = struct{}{}
	}
	n.DescriptorRef = strings.TrimSpace(n.DescriptorRef)
	routeKeys := make(map[string]struct{}, len(n.HTTPRoutes))
	transcodingRouteCount := 0
	httpProxyRouteCount := 0
	for routeIndex := range n.HTTPRoutes {
		normalized, err := normalizeHTTPRoute(n.HTTPRoutes[routeIndex], routeIndex)
		if err != nil {
			return err
		}
		// 原生 HTTP route 没有 full_method；只有 gRPC 转码 route 才需要和 methods[] 交叉校验。
		if normalized.FullMethod != "" {
			if _, exists := methodSet[normalized.FullMethod]; !exists {
				return fmt.Errorf("http_routes[%d].full_method is not declared in methods: %s", routeIndex, normalized.FullMethod)
			}
			transcodingRouteCount++
		} else {
			httpProxyRouteCount++
		}
		routeKey := normalized.HTTPMethod + " " + normalized.Path
		if _, exists := routeKeys[routeKey]; exists {
			return fmt.Errorf("duplicate http route: %s", routeKey)
		}
		routeKeys[routeKey] = struct{}{}
		n.HTTPRoutes[routeIndex] = normalized
	}
	// ServiceNode 是最终注册 payload，也要防止调用方绕过 manifest 校验后提交混合语义 route。
	if transcodingRouteCount > 0 && httpProxyRouteCount > 0 {
		return fmt.Errorf("http_routes must not mix grpc transcoding and http proxy routes")
	}
	if strings.EqualFold(strings.TrimSpace(n.Protocol), "http") && n.DescriptorRef != "" {
		return errors.New("descriptor_ref must be empty for protocol=http")
	}
	if httpProxyRouteCount > 0 && transcodingRouteCount == 0 && n.DescriptorRef != "" {
		return errors.New("descriptor_ref must be empty when only http proxy routes are present")
	}
	// 最终注册 payload 允许纯 gRPC 服务携带 descriptor_ref；go-consul 只在转码 route 存在时强制要求它。
	if err := validateGatewayDescriptorRef(n.DescriptorRef, transcodingRouteCount > 0); err != nil {
		return err
	}
	return nil
}

// BuildDrainRequest 从约定好的 ServiceNode 直接导出 sidecar-agent 摘流请求。
func (n *ServiceNode) BuildDrainRequest(gracePeriod string) DrainRequest {
	// 如果节点或服务配置为空，则只保留摘流宽限期。
	if n == nil || n.ServiceOptions == nil {
		return DrainRequest{GracePeriod: gracePeriod}
	}
	// 正常情况下从固定节点模型中提取 sidecar 摘流所需字段。
	return DrainRequest{
		// 透传应用 ID，帮助 sidecar 唯一定位业务应用。
		AppId: n.App.Id,
		// 透传应用实例 ID，帮助 sidecar 唯一定位业务实例。
		AppInstanceId: n.App.InstanceId,
		// 透传调用方指定的摘流宽限期。
		GracePeriod: gracePeriod,
	}
}

// BuildDeregisterRequest 从约定好的 ServiceNode 直接导出 sidecar-agent 注销请求。
func (n *ServiceNode) BuildDeregisterRequest() DeregisterRequest {
	// 如果节点或服务配置为空，则返回空注销请求。
	if n == nil || n.ServiceOptions == nil {
		return DeregisterRequest{}
	}
	// 正常情况下从固定节点模型中提取 sidecar 注销所需字段。
	return DeregisterRequest{
		// 透传应用 ID，帮助 sidecar 唯一定位业务应用。
		AppId: n.App.Id,
		// 透传应用实例 ID，帮助 sidecar 唯一定位业务实例。
		AppInstanceId: n.App.InstanceId,
	}
}
