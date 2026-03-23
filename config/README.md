# config

`go-consul/config` 是 `go-micro/config` 的 Consul 实现，提供统一的配置存储与监听能力。

## 能力范围

- `Store`：`Get/GetByQuery/Put/Delete`
- 版本能力：`PutVersion/GetVersion/ListVersions`
- 元信息能力：`GetMeta/PutMeta`
- `Watcher`：`Watch/Unwatch`（基于 Consul blocking query）

## 路径模型

默认命名空间：`/config-center`

按单条配置键生成路径：

- 当前配置：`/{namespace}/{tenant}/{env}/{app}/{group}/{name}/current`
- 版本前缀：`/{namespace}/{tenant}/{env}/{app}/{group}/{name}/versions`
- 版本快照：`/{namespace}/{tenant}/{env}/{app}/{group}/{name}/versions/{version}`
- 元信息：`/{namespace}/{tenant}/{env}/{app}/{group}/{name}/meta`

当 `tenant` 为空时会回退为 `default`。

## 快速开始

```go
package main

import (
	"context"

	consulx "github.com/fireflycore/go-consul"
	consulcfg "github.com/fireflycore/go-consul/config"
	microcfg "github.com/fireflycore/go-micro/config"
)

func main() {
	cli, err := consulx.New(&consulx.Conf{
		Address: "127.0.0.1:8500",
	})
	if err != nil {
		panic(err)
	}

	store, err := consulcfg.NewStore(cli, &consulcfg.Conf{
		Namespace: "/config-center",
	})
	if err != nil {
		panic(err)
	}

	key := microcfg.Key{
		TenantId: "t1",
		Env:      "prod",
		AppId:    "order-service",
		Group:    "db",
		Name:     "primary",
	}

	_ = store.Put(context.Background(), key, &microcfg.Item{
		Version: "v1",
		Content: []byte(`{"dsn":"root:root@tcp(127.0.0.1:3306)/order"}`),
	})

	_, _ = store.Get(context.Background(), key)
}
```
