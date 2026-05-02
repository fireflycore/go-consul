# go-consul/agent 测试覆盖率

## 1. 执行命令

```bash
cd /Users/lhdht/product/firefly/go-consul
go test -count=1 ./agent -coverprofile=agent.cover.out
go tool cover -func=agent.cover.out
```

## 2. 当前整体覆盖率

```text
total: (statements) 96.4%
```

## 3. 当前关键覆盖情况

### 3.1 已较完整覆盖

- `api.go`
  - `Register / Drain / Deregister / Readiness / RuntimeSnapshot / RecoverySnapshot`: `100%`
- `api_model.go`
  - `HealthCheckConfig.Validate`: `100%`
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
  - `Readiness / RuntimeSnapshot / RecoverySnapshot`: `100%`
  - `Drain / Deregister / Shutdown`: `100%`
- `service_node.go`
  - `BuildDNS`: `100%`
  - `Validate`: `95.0%`
  - `BuildDrainRequest / BuildDeregisterRequest`: `100%`
- `http_client.go`
  - `PostJSON / GetJSON / decodeSidecarResponse / isAcceptedStatus`: `100%`
- `grpc_helper.go`
  - `BuildGRPCMethods`: `100%`
- `watch_source.go`
  - `WatchHTTPStatusError.Error / WatchEventParseError.Error / Unwrap`: `100%`
  - `NewWatchSource`: `100%`
  - `Subscribe`: `90.0%`
  - `watchOnce`: `90.4%`
  - `emitWatchEvent`: `94.1%`
  - `trimSSEFieldValue / looksLikeJSON / joinDataLines`: `100%`

### 3.2 仍有提升空间

- `agent.go`
  - `New`: `87.5%`
  - `ConfigureRun`: `90.0%`
- `watch_source.go`
  - `Subscribe / watchOnce` 仍有少量上下文竞态与异常 IO 边缘分支未完全覆盖
- `http_client.go`
  - `doJSONRequest`: `88.5%`
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
github.com/fireflycore/go-consul/agent/agent.go:244:            Readiness      100.0%
github.com/fireflycore/go-consul/agent/agent.go:253:            RuntimeSnapshot100.0%
github.com/fireflycore/go-consul/agent/agent.go:262:            RecoverySnapshot100.0%
github.com/fireflycore/go-consul/agent/agent.go:271:            finishRun      100.0%
github.com/fireflycore/go-consul/agent/api.go:15:               NewApiClient   100.0%
github.com/fireflycore/go-consul/agent/api.go:24:               Register       100.0%
github.com/fireflycore/go-consul/agent/api.go:30:               Drain          100.0%
github.com/fireflycore/go-consul/agent/api.go:36:               Deregister     100.0%
github.com/fireflycore/go-consul/agent/api.go:42:               Readiness      100.0%
github.com/fireflycore/go-consul/agent/api.go:59:               RuntimeSnapshot100.0%
github.com/fireflycore/go-consul/agent/api.go:66:               RecoverySnapshot100.0%
github.com/fireflycore/go-consul/agent/api_model.go:35:         Validate       100.0%
github.com/fireflycore/go-consul/agent/controller.go:34:        NewController  100.0%
github.com/fireflycore/go-consul/agent/controller.go:58:        OnConnected    100.0%
github.com/fireflycore/go-consul/agent/controller.go:108:       OnDisconnected 100.0%
github.com/fireflycore/go-consul/agent/controller.go:126:       Drain          100.0%
github.com/fireflycore/go-consul/agent/controller.go:131:       Deregister     100.0%
github.com/fireflycore/go-consul/agent/controller.go:136:       Status         100.0%
github.com/fireflycore/go-consul/agent/controller.go:144:       ObserveEvent   100.0%
github.com/fireflycore/go-consul/agent/controller.go:187:       RecordError    100.0%
github.com/fireflycore/go-consul/agent/controller.go:201:       recordErrorLocked               100.0%
github.com/fireflycore/go-consul/agent/controller.go:211:       classifyStatusError             100.0%
github.com/fireflycore/go-consul/agent/controller.go:245:       formatStatusTime100.0%
github.com/fireflycore/go-consul/agent/defaults.go:6:           DefaultSidecarAgentConfig       100.0%
github.com/fireflycore/go-consul/agent/defaults.go:27:          normalizeSidecarAgentConfig     100.0%
github.com/fireflycore/go-consul/agent/error.go:27:             Error          100.0%
github.com/fireflycore/go-consul/agent/error.go:45:             Unwrap         100.0%
github.com/fireflycore/go-consul/agent/error.go:61:             Error          100.0%
github.com/fireflycore/go-consul/agent/error.go:71:             Unwrap         100.0%
github.com/fireflycore/go-consul/agent/error.go:89:             Error          100.0%
github.com/fireflycore/go-consul/agent/error.go:103:            Unwrap         100.0%
github.com/fireflycore/go-consul/agent/error.go:123:            Error          100.0%
github.com/fireflycore/go-consul/agent/grpc_helper.go:10:       BuildGRPCMethods100.0%
github.com/fireflycore/go-consul/agent/http_client.go:40:       NewHttpClient  100.0%
github.com/fireflycore/go-consul/agent/http_client.go:55:       PostJSON       100.0%
github.com/fireflycore/go-consul/agent/http_client.go:61:       GetJSON        100.0%
github.com/fireflycore/go-consul/agent/http_client.go:66:       doJSONRequest  88.5%
github.com/fireflycore/go-consul/agent/http_client.go:111:      decodeSidecarResponse           100.0%
github.com/fireflycore/go-consul/agent/http_client.go:138:      isAcceptedStatus100.0%
github.com/fireflycore/go-consul/agent/runtime.go:62:           NewRunner      100.0%
github.com/fireflycore/go-consul/agent/runtime.go:83:           Run            100.0%
github.com/fireflycore/go-consul/agent/service_node.go:57:      BuildDNS       100.0%
github.com/fireflycore/go-consul/agent/service_node.go:78:      NewServiceNode 83.3%
github.com/fireflycore/go-consul/agent/service_node.go:110:     Validate       95.0%
github.com/fireflycore/go-consul/agent/service_node.go:157:     BuildDrainRequest               100.0%
github.com/fireflycore/go-consul/agent/service_node.go:174:     BuildDeregisterRequest          100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:38:      Error          100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:64:      Error          100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:78:      Unwrap         100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:116:     NewWatchSource 100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:133:     Subscribe      90.0%
github.com/fireflycore/go-consul/agent/watch_source.go:189:     watchOnce      90.4%
github.com/fireflycore/go-consul/agent/watch_source.go:293:     emitWatchEvent 94.1%
github.com/fireflycore/go-consul/agent/watch_source.go:376:     looksLikeJSON  100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:384:     trimSSEFieldValue               100.0%
github.com/fireflycore/go-consul/agent/watch_source.go:394:     joinDataLines  100.0%
total:                                                          (statements)   96.4%
```

## 5. 当前结论

- 当前覆盖率已经达到 `96.4%`
- sidecar 只读状态接口、JSON envelope 解析、`health_check` 校验和扩展状态面都已覆盖
- 剩余空白主要集中在少量创建失败、异常 IO 和个别初始化边缘分支
