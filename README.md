# go-consul

`go-consul` 是 `firefly` 体系中基于 Consul 的基础接入库，当前主要提供两类能力：

- Consul 客户端初始化（根包）
- 轻量服务调用实现（`invocation` 子包）

## 包结构

- `conf.go`：Consul 客户端配置模型
- `core.go`：根据配置创建 `*api.Client`
- `invocation/`：面向 `service -> service` 的轻量服务调用实现

## 当前状态

- `registry` 子包已废弃并移除
- 裸机业务服务统一改走 `go-micro/registry/agent -> sidecar-agent -> consul / envoy`
- `go-consul` 当前只保留 Consul 客户端能力与 `invocation` 调用能力

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

- [invocation/README.md](file:///Users/lhdht/product/firefly/go-consul/invocation/README.md)

## 设计约束

- 业务服务不做服务发现，只做服务注册
- 服务发现能力不再通过 `go-consul/registry` 暴露给业务侧
- Consul 特有能力由 `sidecar-agent` 或仓库内部基础能力承接，不再对外暴露 registry 子包
- 新的服务调用主路径应优先围绕 `invocation` 收敛
