# Baremetal Agent Bridge

`go-consul/agent` 是业务服务与本机 `sidecar-agent` 之间的裸机桥接层。

它不直接处理 Consul 注册中心语义，也不处理 mesh 控制面语义；它只负责把业务服务的 `ServiceNode` 生命周期，稳定映射到本机 `sidecar-agent` 的 `register / drain / deregister / watch` 协议上。

## 当前定位

当前包的核心定位主要有 4 件事：

- 基于 `ServiceOptions + gateway.manifest.json` 构造固定 `ServiceNode`
- 在本地前置校验后，对本机 `sidecar-agent` 发起 `register / drain / deregister`
- 订阅本机 `watch` SSE 流，并在重连后自动重放 `register`
- 统一向业务侧暴露 `Start / Run / Shutdown / Status`

当前对外的唯一主入口是：

- `New(*ServiceOptions, SidecarAgentConfig) (*Agent, error)`

## 核心模型

- `ServiceOptions`
  - 业务服务启动配置输入，包含服务基础信息、协议和端口
- `ServiceNode`
  - 业务服务在裸机场景下的标准节点描述，也是当前包的核心模型；当前会固定输出 `dns`、`methods`、`proto_count` 和 `http_routes`，只有存在 gRPC 转码 route 时才输出 `descriptor_ref`
- `SidecarAgentConfig`
  - 业务侧传给 `agent` 包的运行配置，包括 sidecar 地址、manifest 路径、重连间隔、超时、托管回调等
- `GatewayManifest`
  - `protoc-gen-gateway-manifest` 生成的服务能力文件，默认路径为 `dep/protobuf/gen/gateway.manifest.json`
- `Agent`
  - 对外唯一主对象，统一收口 watch/replay、服务托管、摘流、注销和状态查询

## 业务流程

启动阶段：

1. 业务服务构造 `ServiceOptions`
2. 业务服务随构建产物携带 `gateway.manifest.json`
3. 调用 `New(...)` 创建 `Agent`
4. `Agent` 读取并校验 manifest
5. `Agent` 内部构造 `ServiceNode`
6. `Agent` 内部组装 `ApiClient + WatchSource + Controller + Runner`
7. 调用 `Start(ctx)` 或 `Run(ctx)` 启动 watch/replay 主链

连接恢复阶段：

1. `WatchSource` 从 `/watch` 读取 SSE 事件
2. `Runner` 消费 `connected / heartbeat / disconnected`
3. 收到 `connected` 后触发 `Controller.OnConnected()`
4. `Controller` 使用固定 `ServiceNode` 调 `ApiClient.Register(...)`
5. 成功后更新状态；失败则记录 `register_replay` 错误并等待下一轮重连

退出阶段：

1. 业务服务调用 `Shutdown(ctx)`，或 `Run(ctx)` 在退出路径中自动执行
2. 如果配置了业务 `Shutdown` 回调，`Agent` 会先执行业务侧本地收尾
3. 如果配置了 `GracePeriod`，`Agent` 再向 sidecar 发起 `Drain`
4. 最后执行 `Deregister`

## 对业务服务提供的功能

- 统一构造 `ServiceNode`
- 自动读取 manifest 中的 gRPC `method path`
- 自动透传 manifest 中的 `routes[]`；只有存在 gRPC 转码 route 时才透传 `descriptor_ref`
- 在本地提前校验 sidecar 注册契约，尽早暴露参数问题
- sidecar 连接恢复后的自动重放注册
- 业务服务退出时统一 `drain + deregister`
- 可选托管业务服务自己的 `Serve / Shutdown`
- 暴露统一状态快照 `Status`
- 把 watch 错误、坏帧、重放失败和 sidecar API 失败统一收敛成稳定错误类型

## 对 sidecar-agent 使用的内容

当前 `agent` 包会消费 sidecar-agent 暴露的 4 个接口：

- `POST /register`
  - 请求体：`ServiceNode`
- `POST /drain`
  - 请求体：`DrainRequest`
- `POST /deregister`
  - 请求体：`DeregisterRequest`
- `GET /watch`
  - SSE 长连接，输出 `connected / heartbeat / disconnected`

其中 `POST` 管理接口会统一解析 sidecar 的 JSON envelope：

- `success`
- `code`
- `message`
- `data`
- `generated_at`

也就是说，`go-consul/agent` 对 `sidecar-agent` 不暴露新的业务接口，它只消费 sidecar 的本地管理接口与事件流。

## 当前分层

从代码职责看，当前包分成 4 层：

- 输入建模层
  - `ServiceOptions`
  - `ServiceNode`
  - `SidecarAgentConfig`
- 协议适配层
  - `HttpClient`
  - `ApiClient`
  - `WatchSource`
- 控制与编排层
  - `Controller`
  - `Runner`
- 对外主入口层
  - `Agent`

## 建议接入

### 仅接入 sidecar watch/replay

```go
svcAgent, err := agent.New(serviceOptions, agent.SidecarAgentConfig{
  BaseURL:             "http://127.0.0.1:15010",
  GracePeriod:         "20s",
  GatewayManifestPath: "dep/protobuf/gen/gateway.manifest.json",
})
if err != nil {
  return err
}

errCh := svcAgent.Start(ctx)
go func() {
  for err := range errCh {
    logger.Error(err)
  }
}()
```

### 统一托管业务服务

```go
svcAgent, err := agent.New(serviceOptions, agent.SidecarAgentConfig{
  BaseURL:             "http://127.0.0.1:15010",
  GracePeriod:         "20s",
  GatewayManifestPath: "dep/protobuf/gen/gateway.manifest.json",
  Serve: func(ctx context.Context) error {
    return grpcServer.Serve(listener)
  },
  Shutdown: func(ctx context.Context) error {
    grpcServer.GracefulStop()
    return nil
  },
})
if err != nil {
  return err
}

return svcAgent.Run(ctx)
```

## Gateway Manifest 契约

`go-consul/agent` 只从 manifest 读取服务能力，不再接收业务侧手写的 gRPC 描述清单。

最小结构如下：

```json
{
  "schema": "firefly.gateway.manifest.v1",
  "descriptor_ref": "https://minio.lhdht.cn/descriptor/auth/v0.0.1.pb",
  "services": [
    {
      "name": "acme.auth.v1.AuthService",
      "methods": [
        "/acme.auth.v1.AuthService/Login"
      ]
    }
  ],
  "routes": [
    {
      "http_method": "POST",
      "path": "/v1/auth/login",
      "full_method": "/acme.auth.v1.AuthService/Login"
    }
  ]
}
```

约束：

- `schema` 必须为 `firefly.gateway.manifest.v1`
- `services[].methods[]` 是 gRPC 能力事实源，不能为空
- `routes[]` 表示允许 north-south HTTP 入口访问的 route
- gRPC 转码 route 必须填写 `routes[].full_method`，且该值必须存在于 `services[].methods[]`
- 原生 HTTP proxy route 不填写 `full_method`，可选填写 `upstream_path` 或 `strip_prefix`，二者不能同时配置
- 存在 gRPC 转码 route 时，`descriptor_ref` 必须是可由 api-gateway 拉取的 `http` 或 `https` 地址
- 没有 gRPC 转码 route 时不能携带 `descriptor_ref`
- 未标注 HTTP 的 gRPC method 仍会进入 `methods[]`，但不会生成 HTTP route

## 可观测状态

`Status` 至少包含：

- 当前是否 `connected / registered`
- 最近一次服务名与端口
- 最近一次事件类型、事件 ID、事件时间
- 最近一次连接与断连时间
- 累计断连次数
- `register replay` 成功次数与失败次数
- 最近一次错误分类、错误文本与错误时间

当前错误分类包括：

- `watch_http_status`
- `watch_stream_closed`
- `watch_event_parse`
- `register_replay`
- `sidecar_api`
- `runtime_error`
