package config

import (
	"testing"
	"time"

	microConfig "github.com/fireflycore/go-micro/config"
)

func TestClientCacheHitWhenWatchDisabled(t *testing.T) {
	// 关闭 watch，只保留本地缓存命中行为。
	client := newTestClient(
		microConfig.WithClientCacheEnabled(true),
		microConfig.WithClientWatchMode(microConfig.WatchModeOff),
	)

	// 先手动写入一条缓存，再校验读取能命中同一份快照。
	key := testKey("redis")
	client.putCache(key, &microConfig.Raw{Content: []byte("cached")})

	raw, ok := client.getCache(key)
	if !ok {
		t.Fatal("getCache() = miss, want hit")
	}
	if string(raw.Content) != "cached" {
		t.Fatalf("cached content = %q, want %q", raw.Content, "cached")
	}
}

func TestClientCacheExpiresAfterTTL(t *testing.T) {
	// 使用极短 TTL，直接依赖真实时钟验证失效行为。
	client := newTestClient(
		microConfig.WithClientCacheEnabled(true),
		microConfig.WithClientCacheTTL(10*time.Millisecond),
		microConfig.WithClientWatchMode(microConfig.WatchModeOff),
	)

	key := testKey("redis")
	client.putCache(key, &microConfig.Raw{Content: []byte("cached")})

	// 等待 TTL 过期后再读取，应该看到 miss。
	time.Sleep(20 * time.Millisecond)

	if _, ok := client.getCache(key); ok {
		t.Fatal("getCache() = hit after ttl, want miss")
	}
}

func TestBuildConsulClientScope(t *testing.T) {
	// 用最小 Store 构造 scope 计算输入。
	store := &StoreInstance{}
	store.options = microConfig.NewOptions(microConfig.WithNamespace("/config-center"))

	key := testKey("redis")

	// PerKey 应落到精确 current 路径。
	perKey := buildConsulClientScope(store, microConfig.WatchScopePerKey, key)
	if perKey.key != "key:/config-center/default/app/prod/database/redis/current" {
		t.Fatalf("perKey.key = %q", perKey.key)
	}

	// Group 应落到 group 级前缀。
	group := buildConsulClientScope(store, microConfig.WatchScopeGroup, key)
	if group.prefix != "/config-center/default/app/prod/database/" {
		t.Fatalf("group.prefix = %q", group.prefix)
	}

	// App 应落到 app 级前缀。
	app := buildConsulClientScope(store, microConfig.WatchScopeApp, key)
	if app.prefix != "/config-center/default/app/prod/" {
		t.Fatalf("app.prefix = %q", app.prefix)
	}
}

func newTestClient(opts ...microConfig.ClientOption) *ClientInstance {
	// 先复用正式代码里的默认 ClientOptions，避免测试口径漂移。
	clientOptions := microConfig.NewClientOptions(opts...)
	store := &StoreInstance{
		options: microConfig.NewOptions(microConfig.WithNamespace("/config-center")),
	}

	return &ClientInstance{
		store:    store,
		options:  clientOptions,
		cache:    make(map[string]clientCacheEntry),
		watchers: make(map[string]*clientWatchHandle),
	}
}

func testKey(key string) microConfig.Key {
	// testKey 统一返回一条稳定 key，减少各个用例里的重复样板。
	return microConfig.Key{
		Env:   "prod",
		AppId: "app",
		Group: "database",
		Key:   key,
	}
}
