# config

`go-consul/config` 是 `go-micro/config` 的 Consul 实现，提供最小数据面配置存储与监听能力。

> 当前主线口径：在配置中心主线交付中，`go-consul/config` 对应 IDC / 裸机场景。它与 `go-k8s/config` 共享统一契约，但不是要求同一个运行时产物同时引入两套实现。
>
> 当前版本口径：本包已对齐 `github.com/fireflycore/go-micro@v1.6.2`，已补齐 `Client` 实现；`Store` 保留 `Get / Put / Delete`，`Client` 负责聚合 cache 与共享 watch。
>
> 后续 cache / watch / `manage/client` 重构，统一以设计库 `design/config/plan/go-micro-config-manage-client-refactor-plan.md` 为基线。

## 能力范围

- `Store`：`Get/Put/Delete`
- `Watcher`：`Watch/Unwatch`（基于 Consul blocking query）
- `Store` 构造：`NewStore`
- `Client` 构造：`NewClient`
- `Options` 透传：`Config.BuildOptions`

## 路径模型

默认命名空间：`/config-center`

按单条配置键生成路径：

- 当前配置：`/{namespace}/{tenant}/{app}/{env}/{group}/{name}/current`

当 `tenant` 为空时会回退为 `default`。

> 版本历史、发布流水和元信息游标不再保存在数据面，由控制面数据库统一承接。

## 接入方式

调用方可以按场景选择两种接入方式：

- `NewStore`：基于 Consul 客户端创建数据面 `Store`
- `NewClient`：在 `Store` 之上聚合本地 cache 与共享 watch
- `Config.BuildOptions`：把 Consul 侧配置映射到统一 `microcfg.Options`

推荐：

- 需要统一 cache / watch 行为时，优先使用 `NewClient`
- 只需要最小直连读写能力时，继续使用 `Store`

## Watch 与协程数量

`go-consul/config` 里的后台 watch 是按 `WatchScope` 去重启动的，不是按 `Get` 次数去重，也不是固定按 key 一一对应。

前提：

- 只有 `EnableCache=true`
- 且 `WatchMode=On`
- 且对应 key 至少被 `Client.Get(...)` 成功读取过一次

满足上面条件后，后台才会为该 key 所属 scope 挂 watch。

当前规则：

- `WatchScopeGroup`：同一个 `tenant/app/env/group` 下，不管有多少个 key，只起 `1` 个协程
- `WatchScopeApp`：同一个 `tenant/app/env` 下，不管有多少个 group、多少个 key，只起 `1` 个协程
- `WatchScopePerKey`：一个 key 起 `1` 个协程

例子：

- 10 个 key 都在同一个 `group`，默认只起 `1` 个协程
- 10 个 key 分布在 3 个 `group`，默认起 `3` 个协程
- 10 个 key 都在同一个 app，且显式使用 `WatchScopeApp`，只起 `1` 个协程
- 10 个 key 显式使用 `WatchScopePerKey`，则起 `10` 个协程

说明：

- 当前默认 `WatchScope` 来自 `go-micro/config`，默认值是 `WatchScopeGroup`
- 当前实现还没有额外的 `Close` 生命周期接口，因此 watch goroutine 一旦启动，会跟随进程生命周期持续存在

## 加密语义

- `go-consul/config` 遵循 `go-micro/config` 的统一加密语义。
- `microcfg.Raw.Encrypted=false` 时，读取方执行 `Base64 解码 -> 解压 -> 反序列化`。
- `microcfg.Raw.Encrypted=true` 时，读取方执行 `Base64 解码 -> 解密 -> 解压 -> 反序列化`。
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

	store, err := consulcfg.NewStore(cli, &consulcfg.Config{
		Namespace: "/config-center",
	})
	if err != nil {
		panic(err)
	}

	key := microcfg.Key{
		Namespace: "t1",
		Env:       "prod",
		AppId:     "order-service",
		Group:     "db",
		Key:       "primary",
	}

	_ = store.Put(context.Background(), key, &microcfg.Raw{
		Version: "v1",
		Content: []byte(`{"dsn":"root:root@tcp(127.0.0.1:3306)/order"}`),
	})

	client, err := consulcfg.NewClient(
		store,
		microcfg.WithClientCacheEnabled(true),
		microcfg.WithClientWatchMode(microcfg.WatchModeOn),
	)
	if err != nil {
		panic(err)
	}

	_, _ = client.Get(context.Background(), key)
}
```
