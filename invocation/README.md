# Invocation

`go-consul/invocation` 提供基于 Consul 的裸机服务调用实现。

它的目标是对齐 `go-micro/invocation` 的统一调用模型，而不是继续把业务调用暴露成节点发现问题。

## 包定位

`go-consul/invocation` 适合：

- 使用 Consul 维护服务健康信息的 IDC 环境
- 需要与 `go-consul/agent -> sidecar-agent -> consul / envoy` 主链配合的裸机场景
- 但对外希望统一成 `service -> service` 调用模型的场景

## 当前提供的能力

### Config

`Config` 用于描述 Consul invocation 的公共配置，例如：

- `Namespace`
- `DefaultPort`
- `ClusterDomain`
- `ResolverScheme`
- `PreferServiceDNS`
- `CacheTTL`

### Locator

`Locator` 负责把 `ServiceRef` 解析成 `Target`。

支持两类模式：

#### 1. Service DNS 模式

若已经有稳定服务名，则优先直接返回 service 级 DNS target。

#### 2. Endpoint 模式

从 Consul 健康实例列表读取实例信息，在本地做轻量 endpoint 选择。

这样可以把：

- Consul 内部的 health check
- service meta
- blocking query / watch

继续保留在实现层，同时对外维持统一的调用体验。

### NewConnectionManager

该辅助函数负责把：

- Consul `Locator`
- `go-micro/invocation.ConnectionManager`

拼装成统一连接管理入口。

## 设计约束

- Consul 只作为实现后端
- 裸机注册主链由 `go-consul/agent` 承接，`go-consul/invocation` 只负责调用侧
- 不向业务侧泄漏实例列表与节点模型
- 业务侧始终面向 `ServiceRef`
- OTel 观测链路默认继续依赖 `go-micro/invocation.ConnectionManager`

## 当前进度

当前已经完成：

- `Config`
- `Locator`
- `NewConnectionManager`
- 单元测试

当前尚未做的内容：

- 更复杂的健康权重策略
- 更复杂的 endpoint 选择策略
- 更深的 Authz 接入封装

## 测试

当前包已通过：

- `go test ./...`
- `go vet ./...`
