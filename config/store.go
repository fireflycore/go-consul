package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	microConfig "github.com/fireflycore/go-micro/config"
	"github.com/hashicorp/consul/api"
)

const (
	defaultNamespace = "/config-center"
	defaultTenant    = "default"
)

// StoreInstance 是基于 Consul KV 的统一配置存储实现。
type StoreInstance struct {
	// client 是外部注入的 Consul 客户端。
	client *api.Client
	// options 保存通用配置参数。
	options *microConfig.Options
	// watchWaitTime 表示 blocking query 的等待窗口。
	watchWaitTime time.Duration

	// watchMu 用于保护 watchCancels 并发读写。
	watchMu sync.Mutex
	// watchCancels 保存 key 对应的取消函数。
	watchCancels map[string]context.CancelFunc
}

// NewStore 基于 Consul 客户端创建配置存储实例。
func NewStore(client *api.Client, config *Config, opts ...microConfig.Option) (*StoreInstance, error) {
	// Consul 客户端为空时直接报错。
	if client == nil {
		return nil, errors.New("consul config: client is nil")
	}

	// 先构建通用 options，再保存到实例。
	var raw *microConfig.Options
	if config != nil {
		raw = config.BuildOptions(opts...)
	} else {
		raw = microConfig.NewOptions(opts...)
	}

	// 计算 watch 等待时长默认值。
	waitTime := 60 * time.Second
	if config != nil && config.WatchWaitTime > 0 {
		waitTime = config.WatchWaitTime
	}

	// 返回初始化完成的实例。
	return &StoreInstance{
		client:        client,
		options:       raw,
		watchWaitTime: waitTime,
		watchCancels:  make(map[string]context.CancelFunc),
	}, nil
}

// Get 按配置键读取当前生效配置。
func (s *StoreInstance) Get(ctx context.Context, key microConfig.Key) (*microConfig.Raw, error) {
	// key 不合法时直接返回统一错误。
	if err := validateKey(key); err != nil {
		return nil, err
	}

	// 基于 options 生成超时上下文，避免慢请求阻塞。
	reqCtx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 读取 current 路径。
	res, _, err := s.client.KV().Get(s.currentKey(key), (&api.QueryOptions{}).WithContext(reqCtx))
	if err != nil {
		return nil, err
	}
	// 未命中时返回统一不存在错误。
	if res == nil || len(res.Value) == 0 {
		return nil, microConfig.ErrResourceNotFound
	}

	// 解析配置内容并返回。
	return s.decodeRaw(res.Value)
}

// Put 写入当前生效配置。
func (s *StoreInstance) Put(ctx context.Context, key microConfig.Key, raw *microConfig.Raw) error {
	// key 不合法时直接返回统一错误。
	if err := validateKey(key); err != nil {
		return err
	}
	// raw 为空时直接返回统一错误。
	if raw == nil {
		return microConfig.ErrInvalidRaw
	}

	// 编码配置内容。
	val, err := s.encodeRaw(raw)
	if err != nil {
		return err
	}

	// 使用超时上下文执行写入。
	reqCtx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 写入 current 路径。
	_, err = s.client.KV().Put(&api.KVPair{
		Key:   s.currentKey(key),
		Value: val,
	}, (&api.WriteOptions{}).WithContext(reqCtx))
	return err
}

// Delete 删除当前配置。
func (s *StoreInstance) Delete(ctx context.Context, key microConfig.Key) error {
	// key 不合法时直接返回统一错误。
	if err := validateKey(key); err != nil {
		return err
	}

	// 使用超时上下文执行删除。
	reqCtx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 删除 current 路径。
	_, err := s.client.KV().Delete(s.currentKey(key), (&api.WriteOptions{}).WithContext(reqCtx))
	return err
}

// withTimeout 基于 options.Timeout 包装上下文。
func (s *StoreInstance) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	// 空上下文回退为 Background。
	if ctx == nil {
		ctx = context.Background()
	}
	// 无超时配置时返回可取消上下文。
	if s.options == nil || s.options.Timeout <= 0 {
		return context.WithCancel(ctx)
	}
	// 使用配置的超时时间创建上下文。
	return context.WithTimeout(ctx, s.options.Timeout)
}

// namespace 返回最终使用的命名空间。
func (s *StoreInstance) namespace() string {
	// 默认命名空间用于避免空路径。
	ns := defaultNamespace
	// 允许 options 覆盖默认命名空间。
	if s.options != nil && s.options.Namespace != "" {
		ns = s.options.Namespace
	}
	// 统一前导斜杠风格。
	if !strings.HasPrefix(ns, "/") {
		ns = "/" + ns
	}
	// 去掉尾部斜杠，避免重复分隔符。
	return strings.TrimRight(ns, "/")
}

// normalizeTenant 返回租户路径片段。
func normalizeTenant(tenant string) string {
	// 未设置租户时使用默认租户路径。
	if strings.TrimSpace(tenant) == "" {
		return defaultTenant
	}
	// 去除首尾空格后返回。
	return strings.TrimSpace(tenant)
}

// currentKey 生成 current 配置路径。
func (s *StoreInstance) currentKey(key microConfig.Key) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s/current",
		s.namespace(), normalizeTenant(key.TenantId), key.Env, key.AppId, key.Group, key.Name,
	)
}

// encodeRaw 对配置内容做编码。
func (s *StoreInstance) encodeRaw(raw *microConfig.Raw) ([]byte, error) {
	// 优先使用调用方注入的编解码器。
	if s.options != nil && s.options.Codec != nil {
		return s.options.Codec.Marshal(raw)
	}
	// 默认使用 JSON 编码。
	return json.Marshal(raw)
}

// decodeRaw 对配置内容做解码。
func (s *StoreInstance) decodeRaw(data []byte) (*microConfig.Raw, error) {
	// 准备承载结果对象。
	raw := new(microConfig.Raw)
	// 优先使用调用方注入的编解码器。
	if s.options != nil && s.options.Codec != nil {
		if err := s.options.Codec.Unmarshal(data, raw); err != nil {
			return nil, err
		}
		return raw, nil
	}
	// 默认使用 JSON 解码。
	if err := json.Unmarshal(data, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// validateKey 校验配置键合法性。
func validateKey(key microConfig.Key) error {
	// Env 为空时视为无效 key。
	if strings.TrimSpace(key.Env) == "" {
		return microConfig.ErrInvalidKey
	}
	// AppId 为空时视为无效 key。
	if strings.TrimSpace(key.AppId) == "" {
		return microConfig.ErrInvalidKey
	}
	// Group 为空时视为无效 key。
	if strings.TrimSpace(key.Group) == "" {
		return microConfig.ErrInvalidKey
	}
	// Name 为空时视为无效 key。
	if strings.TrimSpace(key.Name) == "" {
		return microConfig.ErrInvalidKey
	}
	// key 校验通过。
	return nil
}
