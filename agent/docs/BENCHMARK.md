# go-consul/agent 基准测试文档

## 1. 目标

本基准测试聚焦 `go-consul/agent` 的微观热点路径，主要回答 3 个问题：

- `ServiceNode` 的构造成本如何
- gRPC method path 提取成本如何
- watch 事件帧解析和 register replay 的单次成本如何

## 2. 测试文件

基准测试代码位于：

- `/Users/lhdht/product/firefly/go-consul/agent/benchmark_test.go`

主要覆盖：

- `BenchmarkBuildGRPCMethods`
- `BenchmarkNewServiceNode`
- `BenchmarkControllerOnConnected`
- `BenchmarkEmitWatchEvent`

## 3. 执行命令

```bash
cd /Users/lhdht/product/firefly/go-consul
go test ./agent -run=^$ -bench=. -benchmem
```

## 4. 测试环境

- `goos`: `darwin`
- `goarch`: `arm64`
- `cpu`: `Apple M2`

## 5. 当前结果

```text
BenchmarkBuildGRPCMethods/small-8                4313587               278.6 ns/op           384 B/op          9 allocs/op
BenchmarkBuildGRPCMethods/medium-8                164324              7304 ns/op    6400 B/op        129 allocs/op
BenchmarkBuildGRPCMethods/large-8                  17858             67719 ns/op   58945 B/op       1025 allocs/op
BenchmarkNewServiceNode-8                         155965              8260 ns/op    6875 B/op        138 allocs/op
BenchmarkControllerOnConnected-8                13052203                91.59 ns/op           31 B/op          1 allocs/op
BenchmarkEmitWatchEvent/structured-8             1487246               799.9 ns/op           352 B/op          9 allocs/op
BenchmarkEmitWatchEvent/legacy-8                 5734674               209.4 ns/op           256 B/op          6 allocs/op
```

## 6. 结果解读

- `Controller.OnConnected()` 的单次重放成本很低，说明固定 `ServiceNode + Client` 的重放路径很轻。
- `emitWatchEvent()` 在结构化 JSON 帧下的成本主要来自 JSON 反序列化和对象填充。
- 旧版兼容帧 `data: ok` 明显更便宜，说明当前 SSE 结构化协议成本主要落在 JSON 解析上。
- `buildGRPCMethods()` 和 `NewServiceNode()` 的成本会随着服务数和方法数线性上升，这是当前最直观的构造热点。

## 7. 本轮优化收益

- `buildGRPCMethods/small`
  - `769.5 ns/op -> 278.6 ns/op`
  - `28 allocs/op -> 9 allocs/op`
- `buildGRPCMethods/large`
  - `121976 ns/op -> 67719 ns/op`
  - `3084 allocs/op -> 1025 allocs/op`
- `NewServiceNode`
  - `14570 ns/op -> 8260 ns/op`
  - `401 allocs/op -> 138 allocs/op`
- `emitWatchEvent/structured`
  - `1180 ns/op -> 799.9 ns/op`
  - `10 allocs/op -> 9 allocs/op`

本轮主要优化点：

- 预先统计 method 数量并一次性分配切片容量
- 用字符串拼接替代 `fmt.Sprintf`
- 为 SSE `data` 拼接增加单行快速路径
- 用指针池复用结构化 `watchEventPayload`

## 8. 当前结论

- 运行期热路径最轻的是 `register replay`
- 构造期热点主要是 method path 提取
- 事件解析热点主要是结构化 SSE JSON 解码

## 9. 后续建议

- 如果后续业务服务方法数继续增长，可优先关注 `buildGRPCMethods()` 的分配次数。
- 如果 sidecar `/watch` 事件频率提高，可优先关注结构化 JSON 帧的分配和解码成本。
