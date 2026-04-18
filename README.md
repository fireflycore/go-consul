# go-consul

`go-consul` 是 `firefly` 体系中基于 Consul 的基础接入库，当前主要提供两类能力：

- Consul 客户端初始化（根包）
- 裸机 sidecar-agent 桥接能力（`agent` 子包）

## 包结构

- `conf.go`：Consul 客户端配置模型
- `core.go`：根据配置创建 `*api.Client`
- `agent/`：面向裸机 sidecar-agent 的本地桥接层

## 当前状态

- `registry` 子包已废弃并移除
- 裸机业务服务统一改走 `go-consul/agent -> sidecar-agent -> consul / envoy`
- `go-consul` 当前承载 Consul 客户端能力与裸机 `agent` 桥接能力

## 客户端初始化

```go
package main

import (
	consulx "github.com/fireflycore/go-consul"
)

func main() {
	client, err := consulx.New(&consulx.Conf{
		Address:    "127.0.0.1:8500",
		Scheme:     "http",
		Datacenter: "dc1",
	})
	if err != nil {
		panic(err)
	}

	_ = client
}
```

## 调用能力

当前推荐只使用：

- [agent/README.md](file:///Users/lhdht/product/firefly/go-consul/agent/README.md)

## 设计约束

- 业务服务不做服务发现，只做服务注册
- 服务发现能力不再通过 `go-consul/registry` 暴露给业务侧
- Consul 特有能力由 `sidecar-agent` 或仓库内部基础能力承接，不再对外暴露 registry 子包
- 新的服务调用主路径统一收口到 `go-micro/invocation` 的 DNS-only 模型
