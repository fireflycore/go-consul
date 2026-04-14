# config

`go-consul/config` 是 `go-micro/config` 的 Consul 实现，提供最小数据面配置存储与监听能力。

> 当前主线口径：在配置中心主线交付中，`go-consul/config` 对应 IDC / 裸机场景。它与 `go-k8s/config` 共享统一契约，但不是要求同一个运行时产物同时引入两套实现。
>
> 当前版本口径：本包已对齐 `github.com/fireflycore/go-micro@v1.3.6`，`Store` 只保留 `Get / Put / Delete`，监听能力由独立 `Watcher` 接口承载。

## 能力范围

- `Store`：`Get/Put/Delete`
- `Watcher`：`Watch/Unwatch`（基于 Consul blocking query）
- `loader` 辅助：`NewStoreFromLoader`、`LoadConfigFromStore`

## 路径模型

默认命名空间：`/config-center`

按单条配置键生成路径：

- 当前配置：`/{namespace}/{tenant}/{env}/{app}/{group}/{name}/current`

当 `tenant` 为空时会回退为 `default`。

> 版本历史、发布流水和元信息游标不再保存在数据面，由控制面数据库统一承接。

## Loader 辅助

当调用方已经接入 `go-micro/config` 的 `LoaderParams` / `StoreParams` 体系时，可以直接使用：

- `NewStoreFromLoader`：先按 local / remote 规则加载 `consul.Conf`，再创建 `Store`
- `LoadConfigFromStore`：从 `Store` 读取配置并解码为目标类型

## 加密语义

- `go-consul/config` 遵循 `go-micro/config` 的统一加密语义。
- `microcfg.Raw.Encrypted=false` 时，读取方直接解析配置内容。
- `microcfg.Raw.Encrypted=true` 时，读取方必须先解密整份配置内容，再解析目标结构。
- 不做字段级加密；如果只有部分内容需要保护，应拆成独立配置项。

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

	_ = store.Put(context.Background(), key, &microcfg.Raw{
		Version: "v1",
		Content: []byte(`{"dsn":"root:root@tcp(127.0.0.1:3306)/order"}`),
	})

	_, _ = store.Get(context.Background(), key)
}
```
