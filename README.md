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

- [agent/README.md](/Users/lhdht/product/synergy/firefly/golang/go-consul/agent/README.md)

## 设计约束

- 业务服务不做服务发现，只做服务注册
- 服务发现能力不再通过 `go-consul/registry` 暴露给业务侧
- Consul 特有能力由 `sidecar-agent` 或仓库内部基础能力承接，不再对外暴露 registry 子包
- 新的服务调用主路径统一收口到 `go-micro/invocation` 的 DNS-only 模型
- `agent` 子包采用 manifest-first 注册契约，业务服务必须随构建产物提供 `dep/protobuf/gen/gateway.manifest.json`
- HTTP/JSON 入口 route 只能来自 manifest `routes[]`，不能由业务侧手写 gRPC 描述清单推导
- HTTP/JSON -> gRPC 转码 descriptor 由 proto 仓库发布到 `{namespace}/api-gateway/descriptor/current`，业务服务 manifest 和 route document 不携带 service-level descriptor 字段
- 原生 HTTP proxy route 不写 `full_method`，也不能和 gRPC 转码 route 混写
