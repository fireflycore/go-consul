# go-consul/agent 性能测试文档

## 1. 目标

本性能测试关注的是比单次函数调用更偏运行态的场景，重点验证：

- 控制器状态读取在并发下的成本
- 事件观测写入在并发下的成本
- `Runner` 在事件突发场景下的处理能力

## 2. 测试文件

性能场景测试代码位于：

- `/Users/lhdht/product/firefly/go-consul/agent/performance_benchmark_test.go`

主要覆盖：

- `BenchmarkControllerStatusParallel`
- `BenchmarkControllerObserveEventParallel`
- `BenchmarkRunnerProcessEventBurst`

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
BenchmarkControllerStatusParallel-8             13688576                88.17 ns/op            0 B/op          0 allocs/op
BenchmarkControllerObserveEventParallel-8        5219928               225.0 ns/op            63 B/op          2 allocs/op
BenchmarkRunnerProcessEventBurst/16_events-8      182486              6516 ns/op            6042 B/op         95 allocs/op
BenchmarkRunnerProcessEventBurst/128_events-8      32540             36849 ns/op           31715 B/op        393 allocs/op
BenchmarkRunnerProcessEventBurst/512_events-8       8282            140149 ns/op          119806 B/op       1417 allocs/op
```

## 6. 结果解读

- `Controller.Status()` 并发读取几乎没有额外分配，说明状态快照的读路径足够轻。
- `Controller.ObserveEvent()` 在并发写入场景下有少量分配，主要来自时间格式化和字符串写入。
- `RunnerProcessEventBurst` 随事件量增长呈稳定上升，说明当前主链路没有明显异常退化。
- 事件突发处理的主要成本来自：
  - `ConnectionEvent` 处理
  - `Controller` 状态更新
  - `connected / disconnected` 分支切换

## 7. 本轮优化观察

- `RunnerProcessEventBurst/16_events`
  - `7481 ns/op -> 6516 ns/op`
  - `131 allocs/op -> 95 allocs/op`
- `RunnerProcessEventBurst/128_events`
  - `429 allocs/op -> 393 allocs/op`
- `RunnerProcessEventBurst/512_events`
  - `1453 allocs/op -> 1417 allocs/op`
- `ControllerStatusParallel`
  - `96.27 ns/op -> 88.17 ns/op`

这些收益主要来自 `ServiceNode` 构造与 gRPC method path 提取成本下降，以及结构化 watch payload 的指针池复用。

## 8. 适用场景

这些性能测试更适合回答以下问题：

- sidecar `watch` 短时间内连续抖动时，agent 是否会出现明显吞吐下降
- 多 goroutine 频繁读取状态时，是否会引入额外分配
- 高频事件写入下，控制器锁竞争是否会成为明显瓶颈

## 9. 当前结论

- 当前 `agent` 的读状态路径很轻
- 事件观测写路径开销可接受
- `Runner` 在中等规模事件突发下表现稳定，尚未看到明显异常退化

## 10. 后续建议

- 如果未来 `watch` 协议变得更频繁，建议继续关注 `ObserveEvent()` 的分配次数。
- 如果后续增加更多状态字段，建议重新测一次 `Status()` 并发读和事件突发场景。
