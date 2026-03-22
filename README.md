# go-consul

`go-consul` 是 `firefly` 体系中基于 Consul 的基础接入库，当前主要提供两类能力：

- Consul 客户端初始化（根包）
- 服务注册与网关发现（`registry` 子包）

## 包结构

- `conf.go`：Consul 客户端配置模型
- `core.go`：根据配置创建 `*api.Client`
- `registry/`：注册中心实现（注册、注销、发现、监听）

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

## 注册中心能力

`registry` 子包主要面向以下两类角色：

- 微服务进程：使用 `RegisterInstance` 做 `Install/Uninstall`
- 网关进程：使用 `DiscoverInstance` 做方法路由发现

详细使用方式与模型说明见：

- [registry/README.md](file:///Users/lhdht/product/firefly/go-consul/registry/README.md)

## 设计约束

- 业务服务不做服务发现，只做服务注册
- 服务发现能力由网关统一使用
- Consul 特有能力（健康检查、阻塞查询、事件模型）放在 `go-consul/registry` 内部维护
