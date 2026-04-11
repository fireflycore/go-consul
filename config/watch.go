package config

import (
	"context"
	"time"

	microConfig "github.com/fireflycore/go-micro/config"
	"github.com/hashicorp/consul/api"
)

// Watch 监听指定配置键的变更事件。
func (s *StoreInstance) Watch(ctx context.Context, key microConfig.Key) (<-chan microConfig.WatchEvent, error) {
	// key 不合法时直接返回统一错误。
	if err := validateKey(key); err != nil {
		return nil, err
	}
	// 空上下文回退为 Background。
	if ctx == nil {
		ctx = context.Background()
	}

	// 计算 current 路径，并作为 watch 唯一标识。
	currentKey := s.currentKey(key)

	// 为该 watch 创建可取消上下文。
	watchCtx, cancel := context.WithCancel(ctx)

	// 把取消函数记录到 map，供 Unwatch 调用。
	s.watchMu.Lock()
	s.watchCancels[currentKey] = cancel
	s.watchMu.Unlock()

	// 计算输出通道缓冲区大小。
	bufferSize := 8
	if s.options != nil && s.options.WatchBuffer > 0 {
		bufferSize = s.options.WatchBuffer
	}
	out := make(chan microConfig.WatchEvent, bufferSize)

	// 启动异步 blocking query 监听循环。
	go func() {
		// 关闭前移除取消函数，避免泄漏。
		defer func() {
			s.watchMu.Lock()
			delete(s.watchCancels, currentKey)
			s.watchMu.Unlock()
			close(out)
		}()

		// Consul 阻塞查询索引游标。
		var waitIndex uint64
		for {
			// 上下文结束时退出循环。
			select {
			case <-watchCtx.Done():
				return
			default:
			}

			// 构造阻塞查询参数，等待当前 key 的下一次变化。
			options := (&api.QueryOptions{
				WaitIndex: waitIndex,
				WaitTime:  s.watchWaitTime,
			}).WithContext(watchCtx)

			// 发起 blocking query。
			kv, meta, err := s.client.KV().Get(currentKey, options)
			if err != nil {
				// 失败时退避，避免在网络抖动场景下空转。
				timer := time.NewTimer(300 * time.Millisecond)
				select {
				case <-watchCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
					continue
				}
			}

			// 仅当 LastIndex 前进时继续处理。
			if meta == nil || meta.LastIndex == 0 || meta.LastIndex <= waitIndex {
				continue
			}
			waitIndex = meta.LastIndex

			// 把 Consul 结果转换为统一事件。
			event, ok := s.toWatchEvent(key, kv)
			if !ok {
				continue
			}

			// 发送时支持上下文取消，避免阻塞。
			select {
			case <-watchCtx.Done():
				return
			case out <- event:
			}
		}
	}()

	// 返回监听通道。
	return out, nil
}

// Unwatch 取消指定配置键的监听。
func (s *StoreInstance) Unwatch(key microConfig.Key) {
	// key 不合法时直接忽略，保持幂等。
	if err := validateKey(key); err != nil {
		return
	}

	// 查找对应 cancel 并触发取消。
	currentKey := s.currentKey(key)
	s.watchMu.Lock()
	cancel, ok := s.watchCancels[currentKey]
	s.watchMu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
}

// toWatchEvent 把 Consul KV 查询结果转换为统一 watch 事件。
func (s *StoreInstance) toWatchEvent(key microConfig.Key, kv *api.KVPair) (microConfig.WatchEvent, bool) {
	// 删除场景：key 不存在。
	if kv == nil {
		return microConfig.WatchEvent{
			Type: microConfig.EventDelete,
			Key:  key,
		}, true
	}

	// 读取到的数据为空时视为无效事件。
	if len(kv.Value) == 0 {
		return microConfig.WatchEvent{}, false
	}

	// 解析配置内容。
	raw, err := s.decodeRaw(kv.Value)
	if err != nil {
		return microConfig.WatchEvent{}, false
	}

	// 返回统一 put 事件。
	return microConfig.WatchEvent{
		Type: microConfig.EventPut,
		Key:  key,
		Raw:  raw,
	}, true
}
