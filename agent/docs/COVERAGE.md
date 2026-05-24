# go-consul/agent 测试覆盖率

## 1. 执行命令

```bash
cd /Users/lhdht/product/firefly/go-consul
go test -count=1 ./agent -coverprofile=agent.cover.out
go tool cover -func=agent.cover.out
```

## 2. 当前重点覆盖

当前测试围绕 manifest-first 注册契约覆盖以下路径：

- `LoadGatewayManifest`
  - manifest 缺失
  - JSON 解析失败
  - unknown field 拒绝
  - schema 校验
  - `services[].methods[]` 校验
  - `routes[].full_method` 与 `methods[]` 交叉校验
  - `descriptor_ref` HTTP/HTTPS 约束
- `New`
  - 默认 sidecar 配置补齐
  - 默认 manifest 路径补齐
  - manifest 加载失败返回
  - `ServiceNode` 构造与敏感字段脱敏
- `ServiceNode.Validate`
  - 基础服务身份字段
  - 业务端口与权重
  - `methods[]`
  - `descriptor_ref`
  - `http_routes[]`
- `Controller / Runner`
  - register replay
  - drain / deregister
  - watch 连接状态与错误分类

## 3. 执行要求

每次改动注册契约后至少执行：

```bash
go test ./...
```

涉及 manifest loader、watch replay 或 HTTP client 时，建议额外执行：

```bash
go test -count=1 ./agent -cover
```

## 4. 当前结论

- `RawServices` / `grpc.ServiceDesc` 相关测试已经移除。
- manifest 缺失、解析失败或字段冲突都会阻断 `New(...)`。
- HTTP route 只从 manifest `routes[]` 进入 `ServiceNode.http_routes`。
- 未标注 HTTP 的 gRPC method 仍进入 `ServiceNode.methods`，但不会生成 HTTP route。
