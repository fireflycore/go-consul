# go-consul/agent 测试覆盖率

## 1. 执行命令

```bash
cd /Users/lhdht/product/firefly/go-consul
go test ./agent -coverprofile=agent.cover.out
go tool cover -func=agent.cover.out
```

## 2. 当前整体覆盖率

```text
total: (statements) 95.7%
```

## 3. 当前关键覆盖情况

### 3.1 已较完整覆盖

- `api.go`
  - `Register / Drain / Deregister`: `100%`
- `controller.go`
  - `NewController / OnConnected / OnDisconnected / Drain / Deregister / Status`: `100%`
  - `ObserveEvent / RecordError / classifyStatusError`: `100%`
- `defaults.go`
  - `DefaultSidecarAgentConfig / normalizeSidecarAgentConfig`: `100%`
- `error.go`
  - 所有 `Error / Unwrap`：`100%`
- `runtime.go`
  - `NewRunner / Run`: `100%`
- `agent.go`
  - `ConfigureRun`: `90.0%`
  - `Run`: `94.4%`
  - `Drain / Deregister / Shutdown`: `100%`
- `service_node.go`
  - `BuildDNS`: `100%`
  - `BuildDrainRequest / BuildDeregisterRequest`: `100%`
- `grpc_helper.go`
  - `BuildGRPCMethods`: `100%`
- `watch_source.go`
  - `WatchHTTPStatusError.Error / WatchEventParseError.Error / Unwrap`: `100%`
  - `NewWatchSource`: `100%`
  - `Subscribe`: `90.0%`
  - `watchOnce`: `86.5%`
  - `emitWatchEvent`: `94.1%`
  - `trimSSEFieldValue / looksLikeJSON / joinDataLines`: `100%`

### 3.2 仍有提升空间

- `agent.go`
  - `New`: `87.5%`
  - `ConfigureRun`: `90.0%`
- `watch_source.go`
  - `Subscribe / watchOnce` 仍有少量上下文竞态与异常 IO 边缘分支未完全覆盖
- `http_client.go`
  - `PostJSON`: `92.9%`
- `service_node.go`
  - `NewServiceNode`: `83.3%`

## 4. 覆盖率结果原始输出

```text
github.com/fireflycore/go-consul/agent/agent.go:42:             New            87.5%
github.com/fireflycore/go-consul/agent/agent.go:100:            ConfigureRun   90.0%
github.com/fireflycore/go-consul/agent/agent.go:124:            Start          100.0%
github.com/fireflycore/go-consul/agent/agent.go:150:            Run            94.4%
github.com/fireflycore/go-consul/agent/agent.go:198:            Drain          100.0%
github.com/fireflycore/go-consul/agent/agent.go:208:            Deregister     100.0%
github.com/fireflycore/go-consul/agent/agent.go:218:            Shutdown       100.0%
github.com/fireflycore/go-consul/agent/agent.go:234:            Status         100.0%
github.com/fireflycore/go-consul/agent/agent.go:244:            finishRun      100.0%
github.com/fireflycore/go-consul/agent/api.go:14:               NewApiClient   100.0%
github.com/fireflycore/go-consul/agent/api.go:23:               Register       100.0%
github.com/fireflycore/go-consul/agent/api.go:29:               Drain          100.0%
github.com/fireflycore/go-consul/agent/api.go:35:               Deregister     100.0%
github.com/fireflycore/go-consul/agent/controller.go:34:        NewController  100.0%
github.com/fireflycore/go-consul/agent/controller.go:59:        OnConnected    100.0%
github.com/fireflycore/go-consul/agent/controller.go:109:       OnDisconnected 100.0%
github.com/fireflycore/go-consul/agent/controller.go:127:       Drain          100.0%
github.com/fireflycore/go-consul/agent/controller.go:132:       Deregister     100.0%
github.com/fireflycore/go-consul/agent/controller.go:137:       Status         100.0%
github.com/fireflycore/go-consul/agent/controller.go:145:       ObserveEvent   100.0%
github.com/fireflycore/go-consul/agent/controller.go:173:       RecordError    100.0%
github.com/fireflycore/go-consul/agent/controller.go:187:       recordErrorLocked               100.0%
github.com/fireflycore/go-consul/agent/controller.go:197:       classifyStatusError             100.0%
github.com/fireflycore/go-consul/agent/controller.go:226:       formatStatusTime100.0%
github.com/fireflycore/go-consul/agent/defaults.go:6:           DefaultSidecarAgentConfig       100.0%
github.com/fireflycore/go-consul/agent/defaults.go:27:          normalizeSidecarAgentConfig     100.0%
github.com/fireflycore/go-consul/agent/error.go:27:             Error          100.0%
github.com/fireflycore/go-consul/agent/error.go:45:             Unwrap         100.0%
github.com/fireflycore/go-consul/agent/error.go:61:             Error          100.0%
github.com/fireflycore/go-consul/agent/error.go:71:             Unwrap         100.0%
github.com/fireflycore/go-consul/agent/error.go:89:             Error          100.0%
github.com/fireflycore/go-consul/agent/error.go:103:            Unwrap         100.0%
github.com/fireflycore/go-consul/agent/grpc_helper.go:10:       BuildGRPCMethods100.0%
github.com/fireflycore/go-consul/agent/http_client.go:22:       NewHttpClient  100.0%
github.com/fireflycore/go-consul/agent/http_client.go:37:       PostJSON       92.9%
github.com/fireflycore/go-consul/agent/runtime.go:62:           NewRunner      100.0%
github.com/fireflycore/go-consul/agent/runtime.go:83:           Run            100.0%
github.com/fireflycore/go-consul/agent/service_node.go:53:      BuildDNS       100.0%
github.com/fireflycore/go-consul/agent/service_node.go:72:      NewServiceNode 83.3%
github.com/fireflycore/go-consul/agent/service_node.go:102:     BuildDrainRequest               100.0%
github.com/fireflycore/go-consul/agent/service_node.go:121:     BuildDeregisterRequest          100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:38:      Error          100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:64:      Error          100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:78:      Unwrap         100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:116:     NewWatchSource 100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:133:     Subscribe      90.0%
github.com/fireflycore/go-consul/agent/watch_source.go:189:     watchOnce      86.5%
github.com/fireflycore/go-consul/agent/watch_source.go:293:     emitWatchEvent 94.1%
github.com/fireflycore/go-consul/agent/watch_source.go:376:     looksLikeJSON  100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:384:     trimSSEFieldValue               100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:394:     joinDataLines  100.0%
total:                                                          (statements)   95.7%
```

## 5. 当前结论

- 当前覆盖率已经达到 `95.7%`
- 运行主链、watch 事件解析、ServiceNode 构造、Controller 主要路径都已覆盖
- 剩余空白主要集中在少量创建失败、异常 IO 和个别初始化边缘分支
