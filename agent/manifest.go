package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
)

const (
	// GatewayManifestSchema 表示当前 go-consul 支持的 gateway manifest 契约版本。
	GatewayManifestSchema = "firefly.gateway.manifest.v1"
	// DefaultGatewayManifestPath 表示业务服务构建后默认携带的 gateway manifest 路径。
	DefaultGatewayManifestPath = "dep/protobuf/gen/gateway.manifest.json"
)

// GatewayManifest 描述 protoc-gen-gateway-manifest 生成的服务能力文件。
type GatewayManifest struct {
	// Schema 表示 manifest 的契约版本，防止不同版本结构被误读。
	Schema string `json:"schema"`
	// DescriptorRef 表示 api-gateway 加载 protobuf descriptor set 的 HTTP/HTTPS 地址。
	DescriptorRef string `json:"descriptor_ref,omitempty"`
	// Services 表示当前业务服务真正拥有的 gRPC service 与 method 集合。
	Services []GatewayManifestService `json:"services"`
	// Routes 表示被 google.api.http 标注过、允许 HTTP/JSON 入口访问的 gRPC method。
	Routes []HTTPRoute `json:"routes,omitempty"`
}

// GatewayManifestService 描述 manifest 中的单个 gRPC service。
type GatewayManifestService struct {
	// Name 表示完整 protobuf service 名称，例如 acme.auth.v1.AuthService。
	Name string `json:"name"`
	// Methods 表示该 service 下的完整 gRPC method path。
	Methods []string `json:"methods"`
}

// HTTPRoute 描述单条 HTTP/JSON 到 gRPC method 的入口映射。
type HTTPRoute struct {
	// HTTPMethod 表示 HTTP 方法，例如 GET、POST、PUT、DELETE。
	HTTPMethod string `json:"http_method"`
	// Path 表示 HTTP 入口路径。
	Path string `json:"path"`
	// FullMethod 表示该 HTTP route 对应的完整 gRPC method path。
	FullMethod string `json:"full_method"`
}

// LoadGatewayManifest 从磁盘读取、解析并校验 gateway manifest。
func LoadGatewayManifest(path string) (*GatewayManifest, error) {
	// manifest 路径必须明确存在；空路径通常意味着配置链路没有完成初始化。
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return nil, errors.New("gateway manifest path is required")
	}

	// 读取业务服务随构建产物携带的 manifest 文件。
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("read gateway manifest %q: %w", cleanPath, err)
	}

	// 使用严格 JSON 解码，避免拼写错误或旧字段静默进入注册契约。
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()

	// 解码 manifest 的最小结构。
	var manifest GatewayManifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode gateway manifest %q: %w", cleanPath, err)
	}

	// 防止同一个文件中存在多个 JSON 文档导致尾部内容被忽略。
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode gateway manifest %q: trailing json content", cleanPath)
	}

	// 解析完成后立即做规范化和契约校验，失败时不进入 sidecar 注册链路。
	if err := manifest.NormalizeAndValidate(); err != nil {
		return nil, fmt.Errorf("validate gateway manifest %q: %w", cleanPath, err)
	}

	// 返回已经规范化后的 manifest。
	return &manifest, nil
}

// NormalizeAndValidate 规范化 manifest 内容，并校验它是否满足注册契约。
func (m *GatewayManifest) NormalizeAndValidate() error {
	// manifest 本身不能为空。
	if m == nil {
		return errors.New("gateway manifest is required")
	}

	// schema 必须精确匹配当前支持的版本。
	if strings.TrimSpace(m.Schema) != GatewayManifestSchema {
		return fmt.Errorf("gateway manifest schema must be %q", GatewayManifestSchema)
	}

	// 至少要声明一个业务 service，否则无法形成服务能力。
	if len(m.Services) == 0 {
		return errors.New("gateway manifest services must not be empty")
	}

	// 先收集全局 method 集合，供 route.full_method 交叉校验。
	methodSet := make(map[string]struct{})
	// service 名称必须唯一，避免同一个 service 被拆成多段后产生歧义。
	serviceSet := make(map[string]struct{}, len(m.Services))
	// 逐个 service 校验名称和 methods。
	for serviceIndex := range m.Services {
		// service 名称去空白后必须非空。
		serviceName := strings.TrimSpace(m.Services[serviceIndex].Name)
		if serviceName == "" {
			return fmt.Errorf("gateway manifest services[%d].name is required", serviceIndex)
		}
		// 同一份 manifest 中不允许重复声明 service。
		if _, exists := serviceSet[serviceName]; exists {
			return fmt.Errorf("gateway manifest duplicate service: %s", serviceName)
		}
		serviceSet[serviceName] = struct{}{}
		// 把规范化后的名称写回，确保后续注册 payload 稳定。
		m.Services[serviceIndex].Name = serviceName

		// 每个 service 至少要声明一个 method。
		if len(m.Services[serviceIndex].Methods) == 0 {
			return fmt.Errorf("gateway manifest services[%d].methods must not be empty", serviceIndex)
		}

		// 对当前 service 内的方法做去重排序，避免生成物顺序影响注册 payload。
		serviceMethods := make([]string, 0, len(m.Services[serviceIndex].Methods))
		serviceMethodSet := make(map[string]struct{}, len(m.Services[serviceIndex].Methods))
		for methodIndex, method := range m.Services[serviceIndex].Methods {
			// method 必须是 /{full.service.Name}/{MethodName} 形式。
			method = strings.TrimSpace(method)
			if method == "" {
				return fmt.Errorf("gateway manifest services[%d].methods[%d] is required", serviceIndex, methodIndex)
			}
			if !strings.HasPrefix(method, "/") {
				return fmt.Errorf("gateway manifest method must start with /: %s", method)
			}
			if !strings.HasPrefix(method, "/"+serviceName+"/") {
				return fmt.Errorf("gateway manifest method %s does not belong to service %s", method, serviceName)
			}

			// 当前 service 内重复 method 不改变语义，直接按唯一集合收敛。
			if _, exists := serviceMethodSet[method]; exists {
				continue
			}
			serviceMethodSet[method] = struct{}{}
			serviceMethods = append(serviceMethods, method)
			methodSet[method] = struct{}{}
		}

		// 去重后仍需至少保留一个 method。
		if len(serviceMethods) == 0 {
			return fmt.Errorf("gateway manifest services[%d].methods must not be empty", serviceIndex)
		}
		// 排序后写回，保证注册内容稳定可比对。
		sort.Strings(serviceMethods)
		m.Services[serviceIndex].Methods = serviceMethods
	}

	// 全局 method 集合不能为空。
	if len(methodSet) == 0 {
		return errors.New("gateway manifest methods must not be empty")
	}

	// descriptor_ref 去空白后写回，后续校验和注册都使用规范化值。
	m.DescriptorRef = strings.TrimSpace(m.DescriptorRef)

	// route 存在时必须有 descriptor_ref，api-gateway 才能完成 HTTP/JSON 到 gRPC 的转码。
	if err := validateGatewayDescriptorRef(m.DescriptorRef, len(m.Routes) > 0); err != nil {
		return err
	}

	// HTTP method + path 必须唯一，避免入口路由出现冲突。
	routeKeys := make(map[string]struct{}, len(m.Routes))
	for routeIndex := range m.Routes {
		// 规范化单条 route 并校验必填字段。
		route, err := normalizeHTTPRoute(m.Routes[routeIndex], routeIndex)
		if err != nil {
			return err
		}
		// route.full_method 必须来自 services[].methods[]。
		if _, exists := methodSet[route.FullMethod]; !exists {
			return fmt.Errorf("gateway manifest routes[%d].full_method is not declared in services: %s", routeIndex, route.FullMethod)
		}
		// 同一个 HTTP method + path 只能指向一个 gRPC method。
		routeKey := route.HTTPMethod + " " + route.Path
		if _, exists := routeKeys[routeKey]; exists {
			return fmt.Errorf("gateway manifest duplicate http route: %s", routeKey)
		}
		routeKeys[routeKey] = struct{}{}
		// 把规范化后的 route 写回 manifest。
		m.Routes[routeIndex] = route
	}

	// route 顺序不表达语义，排序后更利于 sidecar-agent 判断 route document 是否变化。
	sort.SliceStable(m.Routes, func(left, right int) bool {
		if m.Routes[left].HTTPMethod != m.Routes[right].HTTPMethod {
			return m.Routes[left].HTTPMethod < m.Routes[right].HTTPMethod
		}
		if m.Routes[left].Path != m.Routes[right].Path {
			return m.Routes[left].Path < m.Routes[right].Path
		}
		if m.Routes[left].FullMethod != m.Routes[right].FullMethod {
			return m.Routes[left].FullMethod < m.Routes[right].FullMethod
		}
		return false
	})

	// service 顺序同样不表达语义，按 service 名称排序保持输出稳定。
	sort.SliceStable(m.Services, func(left, right int) bool {
		return m.Services[left].Name < m.Services[right].Name
	})

	// 所有校验通过。
	return nil
}

// MethodPaths 返回 manifest 中所有 gRPC method path 的稳定去重列表。
func (m *GatewayManifest) MethodPaths() []string {
	// 空 manifest 直接返回空列表，调用方的 Validate 会继续兜底。
	if m == nil {
		return nil
	}

	// 使用 map 汇总所有 service 的 method，避免重复透传。
	methodSet := make(map[string]struct{})
	for _, service := range m.Services {
		for _, method := range service.Methods {
			method = strings.TrimSpace(method)
			if method == "" {
				continue
			}
			methodSet[method] = struct{}{}
		}
	}

	// 把唯一集合转换成稳定排序切片。
	methods := make([]string, 0, len(methodSet))
	for method := range methodSet {
		methods = append(methods, method)
	}
	sort.Strings(methods)

	// 返回排序后的完整 method path。
	return methods
}

// HTTPRoutesCopy 返回 manifest HTTP routes 的安全副本。
func (m *GatewayManifest) HTTPRoutesCopy() []HTTPRoute {
	// 空 manifest 或空 route 直接返回 nil，避免注册 payload 出现无意义空数组。
	if m == nil || len(m.Routes) == 0 {
		return nil
	}

	// 拷贝 route 切片，避免 ServiceNode 持有 manifest 内部切片后被外部修改。
	routes := make([]HTTPRoute, len(m.Routes))
	copy(routes, m.Routes)

	// 返回复制后的 route 列表。
	return routes
}

// ServiceCount 返回 manifest 中声明的 service 数量。
func (m *GatewayManifest) ServiceCount() uint {
	// 空 manifest 没有任何 service。
	if m == nil {
		return 0
	}

	// manifest 已经在加载阶段做过去重和校验，这里直接返回长度即可。
	return uint(len(m.Services))
}

// normalizeHTTPRoute 规范化单条 HTTP route 并校验必填字段。
func normalizeHTTPRoute(route HTTPRoute, routeIndex int) (HTTPRoute, error) {
	// HTTP method 统一转为大写，避免大小写差异造成 route document 抖动。
	route.HTTPMethod = strings.ToUpper(strings.TrimSpace(route.HTTPMethod))
	if route.HTTPMethod == "" {
		return HTTPRoute{}, fmt.Errorf("gateway manifest routes[%d].http_method is required", routeIndex)
	}

	// HTTP path 必须是绝对路径。
	route.Path = strings.TrimSpace(route.Path)
	if route.Path == "" {
		return HTTPRoute{}, fmt.Errorf("gateway manifest routes[%d].path is required", routeIndex)
	}
	if !strings.HasPrefix(route.Path, "/") {
		return HTTPRoute{}, fmt.Errorf("gateway manifest route path must start with /: %s", route.Path)
	}

	// full_method 必须是完整 gRPC method path。
	route.FullMethod = strings.TrimSpace(route.FullMethod)
	if route.FullMethod == "" {
		return HTTPRoute{}, fmt.Errorf("gateway manifest routes[%d].full_method is required", routeIndex)
	}
	if !strings.HasPrefix(route.FullMethod, "/") {
		return HTTPRoute{}, fmt.Errorf("gateway manifest route full_method must start with /: %s", route.FullMethod)
	}

	// 返回规范化后的 route。
	return route, nil
}

// validateGatewayDescriptorRef 校验 descriptor_ref 是否满足当前 HTTP 拉取约束。
func validateGatewayDescriptorRef(descriptorRef string, required bool) error {
	// 当业务确实没有 HTTP/JSON route 时，允许 descriptor_ref 为空。
	if strings.TrimSpace(descriptorRef) == "" {
		if required {
			return errors.New("descriptor_ref is required when http routes are present")
		}
		return nil
	}

	// descriptor_ref 第一阶段只允许 HTTP/HTTPS，便于 api-gateway 直接拉取。
	parsed, err := url.Parse(descriptorRef)
	if err != nil {
		return fmt.Errorf("descriptor_ref is invalid: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("descriptor_ref scheme must be http or https: %s", descriptorRef)
	}
	if parsed.Host == "" {
		return fmt.Errorf("descriptor_ref host is required: %s", descriptorRef)
	}

	// descriptor_ref 合法。
	return nil
}
