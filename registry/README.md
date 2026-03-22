# registry

`registry` 包是 `go-consul` 中的注册中心实现，负责将 `go-micro/registry` 的核心模型映射到 Consul。

## 核心能力

- `NewRegister`：创建注册器
- `(*RegisterInstance).Install`：注册当前服务实例
- `(*RegisterInstance).Uninstall`：注销当前服务实例
- `NewDiscover`：创建发现器（网关使用）
- `(*DiscoverInstance).GetService`：按 rpc method 获取服务节点
- `(*DiscoverInstance).Watcher`：持续刷新本地发现缓存
- `(*DiscoverInstance).Unwatch`：停止发现监听

## 配置模型

- `ServiceConf`
  - `InstanceId`、`Namespace`、`Network`、`Kernel`
  - `TTL`、`MaxRetry`、`Weight`
- `GatewayConf`
  - `Network`

`Bootstrap()` 会补齐默认值，避免零值导致运行期问题。

## 元数据映射策略

注册时会把核心信息编码到 Consul `Meta`，包括：

- `env`、`app_id`、`version`
- `network_sn`、`network_external`
- `kernel_language`、`kernel_version`
- `weight`、`run_date`
- `methods`（方法映射 JSON）
- `node`（完整节点 JSON）

健康检查默认使用 TCP 检查，并设置 `DeregisterCriticalServiceAfter` 自动摘除异常实例。

## 快速示例

```go
package main

import (
	consulx "github.com/fireflycore/go-consul"
	"github.com/fireflycore/go-consul/registry"
	micro "github.com/fireflycore/go-micro/registry"
)

func main() {
	client, err := consulx.New(&consulx.Conf{Address: "127.0.0.1:8500"})
	if err != nil {
		panic(err)
	}

	reg, err := registry.NewRegister(client, &micro.Meta{
		Env:     "prod",
		AppId:   "user-service",
		Version: "v1.0.0",
	}, &registry.ServiceConf{
		Network: &micro.Network{Internal: "127.0.0.1:9001"},
		Kernel:  &micro.Kernel{},
		TTL:     10,
		Weight:  100,
	})
	if err != nil {
		panic(err)
	}

	_ = reg.Install(&micro.ServiceNode{
		Methods: map[string]bool{
			"/acme.user.v1.UserService/Login": true,
		},
	})
	defer reg.Uninstall()
}
```

## 网关发现示例

```go
package main

import (
	"context"
	"time"

	consulx "github.com/fireflycore/go-consul"
	"github.com/fireflycore/go-consul/registry"
	micro "github.com/fireflycore/go-micro/registry"
)

func main() {
	client, err := consulx.New(&consulx.Conf{Address: "127.0.0.1:8500"})
	if err != nil {
		panic(err)
	}

	dis, err := registry.NewDiscover(client, &micro.Meta{
		Env: "prod",
	}, &registry.ServiceConf{
		Namespace: "/microservice/demo",
		TTL:       10,
		Network:   &micro.Network{},
		Kernel:    &micro.Kernel{},
	})
	if err != nil {
		panic(err)
	}

	go dis.Watcher()
	defer dis.Unwatch()

	time.Sleep(500 * time.Millisecond)

	nodes, appID, err := dis.GetService("/acme.user.v1.UserService/Login")
	if err != nil {
		panic(err)
	}

	_, _, _ = context.Background(), appID, nodes
}
```

## 说明

- `ServiceEvent` 是 `go-consul/registry` 内部事件模型，不依赖 `go-micro` 的事件定义版本差异。
- 发现器会维护两级索引：`method -> appId`、`appId -> nodes`，供网关快速路由。
