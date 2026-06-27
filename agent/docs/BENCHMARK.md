# go-consul/agent 基准测试文档

## 1. 目标

本基准测试聚焦 `go-consul/agent` 的微观热点路径，主要回答 3 个问题：

- `GatewayManifest.MethodPaths()` 的方法列表提取成本如何
- `ServiceNode` 基于 manifest 的构造成本如何
- watch 事件帧解析和 register replay 的单次成本如何

## 2. 测试文件

基准测试代码位于：

- `/Users/lhdht/product/firefly/go-consul/agent/benchmark_test.go`

主要覆盖：

- `BenchmarkGatewayManifestMethodPaths`
- `BenchmarkNewServiceNode`
- `BenchmarkControllerOnConnected`
- `BenchmarkEmitWatchEvent`

## 3. 执行命令

```bash
cd /Users/lhdht/product/firefly/go-consul
go test ./agent -run=^$ -bench=. -benchmem
```

## 4. 当前契约

当前版本已经移除 `RawServices` / `grpc.ServiceDesc` 注册输入。

构造期能力事实来自 `gateway.manifest.json`：

- `services[].methods[]` 生成 `ServiceNode.methods`
- `routes[]` 生成 `ServiceNode.http_routes`
- Gateway manifest benchmark fixture 只携带 schema、services 和 routes；api-gateway descriptor 由 namespace descriptor current 提供

## 5. 当前结论

- 运行期热路径最轻的是 `register replay`
- 构造期热点主要是 manifest method 列表复制、去重和排序
- 事件解析热点主要是结构化 SSE JSON 解码

## 6. 后续建议

- 如果后续业务服务方法数继续增长，可优先关注 `GatewayManifest.MethodPaths()` 的分配次数。
- 如果 sidecar `/watch` 事件频率提高，可优先关注结构化 JSON 帧的分配和解码成本。
