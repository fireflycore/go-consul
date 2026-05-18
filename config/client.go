package config

import (
	"context"
	"fmt"
	"sync"
	"time"

	microConfig "github.com/fireflycore/go-micro/config"
	"github.com/hashicorp/consul/api"
)

// ClientInstance 是 go-consul/config 的统一配置客户端实现。
// 它在 Store 之上聚合本地缓存与共享 watch，对业务只暴露 Get。
type ClientInstance struct {
	// store 保存底层 Consul Store，所有远端读取最终都落到这里。
	store *StoreInstance
	// options 保存 Client 的运行参数，例如 cache、watch 和超时开关。
	options *microConfig.ClientOptions

	// cacheMu 保护 cache 和 watchers 两张运行时状态表的并发访问。
	cacheMu sync.RWMutex
	// cache 保存当前进程内的配置快照。
	cache map[string]clientCacheEntry
	// watchers 以共享 scope 为键，保证同一 scope 只启动一条 watch 链路。
	watchers map[string]*clientWatchHandle
}

// clientCacheEntry 表示单条缓存记录。
type clientCacheEntry struct {
	// raw 保存配置快照副本，避免调用方改写内部缓存。
	raw *microConfig.Raw
	// expiresAt 只在 watch 关闭时生效；零值表示这条缓存由 watch 持续刷新。
	expiresAt time.Time
}

// clientScope 描述一条共享 watch 的聚合范围。
type clientScope struct {
	// key 用作内部 map 的唯一标识。
	key string
	// prefix 是 Consul 前缀监听范围。
	prefix string
}

// clientWatchHandle 保存某个共享 watch 的运行时状态。
type clientWatchHandle struct {
	// client 回指上层实例，便于 watch 线程刷新缓存。
	client *ClientInstance
	// scope 描述当前 handle 负责的监听范围。
	scope clientScope

	// stateMu 保护 keys 的并发读写。
	stateMu sync.RWMutex
	// keys 保存当前 scope 下已经被 Get 触达过的具体配置键。
	keys map[string]microConfig.Key
}

// NewClient 基于 Store 构造统一配置客户端。
func NewClient(store *StoreInstance, opts ...microConfig.ClientOption) (*ClientInstance, error) {
	// 没有底层 Store 时无法读取远端配置，直接返回统一错误。
	if store == nil {
		return nil, microConfig.ErrStoreIsNil
	}

	// 先收敛 Client 运行参数，再初始化运行时状态。
	raw := microConfig.NewClientOptions(opts...)
	client := &ClientInstance{
		store:    store,
		options:  raw,
		cache:    make(map[string]clientCacheEntry),
		watchers: make(map[string]*clientWatchHandle),
	}
	return client, nil
}

// Get 按配置键读取当前可用配置。
func (c *ClientInstance) Get(ctx context.Context, key microConfig.Key) (*microConfig.Raw, error) {
	// 先复用 Store 的 key 校验规则，避免缓存和远端读取出现口径不一致。
	if err := validateKey(key); err != nil {
		return nil, err
	}

	// 开启缓存时优先命中本地快照，减少对 Consul 的直接读取压力。
	if c.options.EnableCache {
		if raw, ok := c.getCache(key); ok {
			return raw, nil
		}
	}

	loadCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	// 缓存 miss 后回落到底层 Store.Get。
	raw, err := c.store.Get(loadCtx, key)
	if err != nil {
		return nil, err
	}

	// 远端读取成功后再回填缓存，并按需挂载共享 watch。
	if c.options.EnableCache {
		c.putCache(key, raw)
		if c.options.WatchMode == microConfig.WatchModeOn {
			c.ensureWatch(key)
		}
	}

	return cloneRaw(raw), nil
}

// withTimeout 为单次底层读取派生超时上下文，避免慢请求长期阻塞 Get。
func (c *ClientInstance) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	// 调用方未传 ctx 时回退到 Background，保持接口可直接调用。
	if ctx == nil {
		ctx = context.Background()
	}
	// 未配置超时时只返回可取消上下文，避免额外强制截止时间。
	if c.options == nil || c.options.Timeout <= 0 {
		return context.WithCancel(ctx)
	}
	// 配置了超时时，统一在这里收敛读取上界。
	return context.WithTimeout(ctx, c.options.Timeout)
}

// getCache 尝试命中本地缓存，并顺手清理已过期记录。
func (c *ClientInstance) getCache(key microConfig.Key) (*microConfig.Raw, bool) {
	// 同一 key 在 cache、watch 和删除路径里都必须落到同一缓存键。
	cacheKey := c.store.currentKey(key)

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	// 未命中时直接返回 false，让调用方继续走远端读取。
	entry, ok := c.cache[cacheKey]
	if !ok {
		return nil, false
	}
	// 关闭 watch 时由 TTL 驱动失效；过期后立即删除，避免脏命中。
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		delete(c.cache, cacheKey)
		return nil, false
	}
	// 命中时返回副本，避免外部修改内部缓存对象。
	return cloneRaw(entry.raw), true
}

// putCache 写入或更新一条缓存记录。
func (c *ClientInstance) putCache(key microConfig.Key, raw *microConfig.Raw) {
	// 关闭缓存或远端返回为空时无需落本地状态。
	if !c.options.EnableCache || raw == nil {
		return
	}

	cacheKey := c.store.currentKey(key)
	// 写入缓存前先复制一份，避免共享底层切片和 map。
	entry := clientCacheEntry{raw: cloneRaw(raw)}
	// watch 关闭时，缓存只能依赖 TTL 失效，因此需要写过期时间。
	if c.options.WatchMode != microConfig.WatchModeOn {
		entry.expiresAt = time.Now().Add(c.options.CacheTTL)
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	// 每次写入前先清掉过期项，避免长时间累积无效数据。
	c.evictExpiredLocked()
	// 新键进入缓存前再检查容量上限，保持条目数受控。
	if _, ok := c.cache[cacheKey]; !ok {
		c.evictOverflowLocked()
	}
	// 最终把新快照写回缓存表。
	c.cache[cacheKey] = entry
}

// deleteCacheByPath 在 watch 观察到删除或空值时驱逐缓存。
func (c *ClientInstance) deleteCacheByPath(path string) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	delete(c.cache, path)
}

// evictExpiredLocked 清理所有已过期的 TTL 缓存项。
func (c *ClientInstance) evictExpiredLocked() {
	now := time.Now()
	for key, entry := range c.cache {
		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			delete(c.cache, key)
		}
	}
}

// evictOverflowLocked 在容量超限时驱逐一条记录，避免缓存无限增长。
func (c *ClientInstance) evictOverflowLocked() {
	// 未设置上限，或当前仍未超量时无需驱逐。
	if c.options.CacheMaxEntries <= 0 || len(c.cache) < c.options.CacheMaxEntries {
		return
	}
	// 当前实现采用最简单的随机 map 首项淘汰；后续如有需要可再替换为 LRU。
	for key := range c.cache {
		delete(c.cache, key)
		return
	}
}

// ensureWatch 保证当前 key 所属 scope 已经挂上一条共享 watch。
func (c *ClientInstance) ensureWatch(key microConfig.Key) {
	// 先计算共享 scope，再取出具体缓存键，后续要同时登记。
	scope := buildConsulClientScope(c.store, c.options.WatchScope, key)
	cacheKey := c.store.currentKey(key)

	c.cacheMu.Lock()
	handle, ok := c.watchers[scope.key]
	if !ok {
		// 第一次命中该 scope 时创建 handle，并在解锁后启动后台 watch。
		handle = &clientWatchHandle{
			client: c,
			scope:  scope,
			keys:   map[string]microConfig.Key{cacheKey: key},
		}
		c.watchers[scope.key] = handle
		c.cacheMu.Unlock()
		c.startWatch(handle)
		return
	}
	c.cacheMu.Unlock()

	// 已有共享 watch 时，只把具体 key 挂到该 handle 管理即可。
	handle.stateMu.Lock()
	handle.keys[cacheKey] = key
	handle.stateMu.Unlock()
}

// buildConsulClientScope 把抽象 WatchScope 映射为 Consul 前缀监听范围。
func buildConsulClientScope(store *StoreInstance, scope microConfig.WatchScope, key microConfig.Key) clientScope {
	// base 是 app 级共享监听的公共前缀。
	base := fmt.Sprintf("%s/%s/%s", store.namespace(key.Namespace), key.AppId, key.Env)
	switch scope {
	case microConfig.WatchScopePerKey:
		// PerKey 模式只监听当前配置的 current 路径。
		path := store.currentKey(key)
		return clientScope{key: "key:" + path, prefix: path}
	case microConfig.WatchScopeApp:
		// App 模式监听整个 app 下的全部 group。
		return clientScope{key: "app:" + base + "/", prefix: base + "/"}
	default:
		// Group 是默认模式，在成本和粒度之间做折中。
		prefix := base + "/" + key.Group + "/"
		return clientScope{key: "group:" + prefix, prefix: prefix}
	}
}

// startWatch 启动后台 prefix watch，并在每次索引推进后刷新缓存快照。
func (c *ClientInstance) startWatch(handle *clientWatchHandle) {
	go func() {
		// waitIndex 记录上次看到的 Consul 索引，只消费真正前进的变化。
		var waitIndex uint64
		for {
			// Consul 侧使用 blocking query 做长轮询。
			options := (&api.QueryOptions{
				WaitIndex: waitIndex,
				WaitTime:  c.store.watchWaitTime,
			}).WithContext(context.Background())

			// 按 scope 前缀一次拉取快照，避免一 key 一条阻塞查询。
			pairs, meta, err := c.store.client.KV().List(handle.scope.prefix, options)
			if err != nil {
				// 网络抖动时做短暂退避，避免空转打满 CPU。
				time.Sleep(300 * time.Millisecond)
				continue
			}
			// 没有索引推进时说明还没有新事件，继续下一轮阻塞查询。
			if meta == nil || meta.LastIndex == 0 || meta.LastIndex <= waitIndex {
				continue
			}
			waitIndex = meta.LastIndex
			// 索引前进后，把当前 scope 的快照应用回本地缓存。
			c.applyConsulSnapshot(handle, pairs)
		}
	}()
}

// applyConsulSnapshot 把一轮 prefix 查询结果映射为缓存刷新动作。
func (c *ClientInstance) applyConsulSnapshot(handle *clientWatchHandle, pairs api.KVPairs) {
	// 先把返回结果整理成 path -> pair，方便按已登记 key 做快速查找。
	pairMap := make(map[string]*api.KVPair, len(pairs))
	for _, pair := range pairs {
		if pair == nil {
			continue
		}
		pairMap[pair.Key] = pair
	}

	// 复制一份当前 handle 管理的 key 集合，避免长时间持有锁做解码。
	handle.stateMu.Lock()
	keys := make(map[string]microConfig.Key, len(handle.keys))
	for path, key := range handle.keys {
		keys[path] = key
	}
	handle.stateMu.Unlock()

	for path, key := range keys {
		// 快照里已经不存在该 key，或值为空时，说明缓存应被删除。
		pair, ok := pairMap[path]
		if !ok || pair == nil || len(pair.Value) == 0 {
			c.deleteCacheByPath(path)
			continue
		}

		// 解码失败时跳过该条，避免单个坏数据中断整批刷新。
		raw, err := c.store.decodeRaw(pair.Value)
		if err != nil {
			continue
		}
		// 解码成功后把最新快照覆盖进缓存。
		c.putCache(key, raw)
	}
}

// cloneRaw 返回 Raw 的深拷贝，确保缓存与调用方之间没有共享可变状态。
func cloneRaw(raw *microConfig.Raw) *microConfig.Raw {
	if raw == nil {
		return nil
	}

	// 先复制结构体值，再分别复制引用字段。
	dst := *raw
	if raw.Content != nil {
		dst.Content = append([]byte(nil), raw.Content...)
	}
	if raw.Meta != nil {
		dst.Meta = make(map[string]string, len(raw.Meta))
		for key, value := range raw.Meta {
			dst.Meta[key] = value
		}
	}
	return &dst
}
