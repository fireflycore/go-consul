# Baremetal Agent Bridge

`go-consul/agent` 用于承接业务服务与本机 `sidecar-agent` 的联动能力。

## 定位

这个包是当前 Firefly **裸机版本的主路径**。

它不直接对接 Consul，也不直接承担服务发现职责。

它只负责：

- 持有业务服务的标准注册描述
- 对本机 `sidecar-agent` 发起 `register`
- 在本地连接恢复后自动重放注册
- 兼容结构化 SSE `connected / heartbeat` 事件并在连接断开后自动重连
- 把非 200、流关闭与结构化坏帧收敛成可分类错误
- 把 register replay 失败包装成带服务名与端口的可分类错误
- 对外提供统一的 `drain` / `deregister` 入口

## 设计目标

- 不让业务服务自己轮询 agent
- 不让业务服务直接处理 agent 重启逻辑
- 把“注册描述缓存 + 连接恢复后重放”收敛到核心库
- 给 v2.2 的 agent lifecycle 机制提供统一接入点
- 把业务服务和 `consul / envoy` 的耦合统一收口到 `sidecar-agent`

## 当前已落地范围

当前已经提供可直接接入的通用模型、契约与控制器实现：

- `RegisterRequest`
- `DrainRequest`
- `DeregisterRequest`
- `DescriptorProvider`
- `Client`
- `Controller`
- `ConnectionEvent`
- `EventSource`
- `Runner`
- `JSONHTTPClient`
- `WatchSource`
- `LocalRuntime`
- `ServiceRegistration`
- `ServiceRegistrationProvider`
- `GRPCDescriptorOptions`
- `NewServiceRegistrationFromGRPC(...)`
- `NewServiceLifecycleFromGRPC(...)`
- `ServiceLifecycle`
- `ManagedServer`

暂不包含：

- 与业务框架自动启动集成的更深封装
- 比当前 `watch` 机制更强约束的 lease / stream 协议

## 当前边界

当前默认假设：

```text
业务服务
  → go-consul/agent
  → 本机 sidecar-agent
  → sidecar-agent 对接 consul / envoy
```

因此：

- 业务服务不再直接对接 `go-consul/registry`
- 业务服务不再直接感知 `envoy`
- `go-consul/agent` 只处理本机 agent 生命周期，不处理云原生 mesh 语义

## K8s 说明

这个包是**裸机专用桥接层**。

在 K8s 中应视为：

- 不进入业务服务主链
- 不承担注册/摘流/注销职责
- 不复用本机 `sidecar-agent + consul + envoy` 这一套运行模型

也就是说：

- 裸机走 `go-consul/agent`
- K8s 走 `k8s + mesh + go-micro/invocation`

## 后续演进

后续可在此基础上继续补：

- 更强约束的本地长连接协议
- 连接状态订阅
- 注册重放退避策略
- 与 go-micro 启动钩子集成

## 已冻结的下一批事项

为避免后续上下文丢失，当前已明确下一批优先事项如下：

1. 补 `watch` 非 200、坏帧、空帧、EOF 与退避重连测试。
2. 补自动重放失败时的错误分类与更清晰日志。
3. 继续收口 `LocalRuntime / ServiceLifecycle / ManagedServer` 与业务启动钩子的集成边界。
4. 保持 `connected / heartbeat` 新协议与旧版兼容帧的双向兼容测试。

## 建议接入方式

业务服务若已经在使用本地服务节点模型，可优先复用：

- `ServiceRegistration`
- `ServiceRegistrationProvider`
- `NewLocalRuntimeFromServiceRegistration(...)`

这样可以直接把已有服务元信息映射成 sidecar-agent 的注册请求，减少重复拼装代码。

```go
node := &agent.ServiceNode{
  Weight: 100,
  Methods: map[string]bool{
    "/acme.auth.v1.AuthService/Login": true,
  },
  Kernel: &agent.ServiceKernel{
    Language: "go",
    Version:  "go-micro/v1.12.0",
  },
  Meta: &agent.ServiceMeta{
    AppId: "10001",
  },
}

lifecycle, err := agent.NewServiceLifecycleFromServiceRegistration(agent.ServiceRegistration{
  ServiceName: "auth",
  Namespace:   "default",
  DNS:         "auth.default.svc.cluster.local",
  Env:         "prod",
  Port:        9090,
  Protocol:    "grpc",
  Version:     "v1.0.0",
  Node:        node,
}, agent.DefaultLocalRuntimeOptions(""), agent.LifecycleOptions{
  GracePeriod: "20s",
})
if err != nil {
  return err
}

errCh := lifecycle.Start(ctx)

go func() {
  for err := range errCh {
    logger.Error(err)
  }
}()
```

如果业务服务当前更接近 “`grpc.ServiceDesc + agent.ServiceOptions`” 这类输入，也可以直接使用：

- `GRPCDescriptorOptions`
- `NewServiceLifecycleFromGRPC(...)`

它会自动：

- 解析 `grpc.ServiceDesc` 中的完整 method path
- 复用 `ServiceOptions` 中的 `weight / kernel / instance_id`
- 组装 sidecar-agent 所需的标准注册描述

如果你希望把“业务服务启动/退出”和“agent 注册/摘流/注销”统一收敛成一个入口，还可以继续使用：

- `ServiceLifecycle`
- `ManagedServer`

这样业务侧可以把：

- 本地 agent 连接恢复后的自动重放 register
- 退出时的 `drain + deregister`
- 业务服务自己的 `serve + shutdown`

统一收敛到一个 `Run(ctx)` 入口。
