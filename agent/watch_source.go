package agent

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// WatchSource 基于 sidecar-agent 的 `/watch` 长连接接口输出连接事件。
type WatchSource struct {
	// watchURL 表示本机 sidecar-agent 的 watch 接口地址。
	watchURL string
	// client 表示实际发起长连接请求的 HTTP 客户端。
	client *http.Client
	// reconnectInterval 表示断连后的重连间隔。
	reconnectInterval time.Duration
}

// NewWatchSource 创建一个新的长连接事件源。
func NewWatchSource(watchURL string, reconnectInterval time.Duration) *WatchSource {
	// 对 watch 地址做最小清洗，避免前后空白影响请求。
	cleanWatchURL := strings.TrimSpace(watchURL)
	// 返回一个使用无限读超时的最小客户端，以支持长连接流式读取。
	return &WatchSource{
		watchURL: cleanWatchURL,
		client: &http.Client{
			Timeout: 0,
		},
		reconnectInterval: reconnectInterval,
	}
}

// Subscribe 启动后台重连循环，并把连接状态变化转换成事件流。
func (s *WatchSource) Subscribe(ctx context.Context) (<-chan ConnectionEvent, error) {
	// watch 地址为空时直接返回错误，避免后台空跑。
	if s.watchURL == "" {
		return nil, errors.New("watch url is required")
	}
	// 创建事件输出通道，供 Runner 持续消费。
	events := make(chan ConnectionEvent, 8)
	// 启动后台协程维持长连接与重连逻辑。
	go func() {
		// 协程退出前关闭事件通道，通知上游运行器结束消费。
		defer close(events)
		for {
			// 若外层上下文已结束，则直接退出重连循环。
			if ctx.Err() != nil {
				return
			}
			// 发起一轮 watch 连接，并把连接结果透传成事件。
			if err := s.watchOnce(ctx, events); err != nil {
				// 只有在上下文未取消时才继续发出断连事件。
				if ctx.Err() == nil {
					select {
					case <-ctx.Done():
						return
					case events <- ConnectionEvent{Connected: false, Err: err}:
					}
				}
			}
			// 在下一轮重连前等待固定退避，避免打满本地 agent。
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.reconnectInterval):
			}
		}
	}()
	// 返回事件通道给上层运行器。
	return events, nil
}

// watchOnce 建立一轮 watch 连接，并在连接断开前持续阻塞读取。
func (s *WatchSource) watchOnce(ctx context.Context, events chan<- ConnectionEvent) error {
	// 创建带上下文的 GET 请求，确保取消时能及时打断阻塞读取。
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.watchURL, nil)
	if err != nil {
		return err
	}
	// 明确告诉服务端当前连接期望接收 SSE。
	request.Header.Set("Accept", "text/event-stream")
	// 发起连接请求。
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	// 在函数退出时关闭响应体，释放底层连接。
	defer response.Body.Close()
	// 仅允许 200 成功响应进入长连接读取阶段。
	if response.StatusCode != http.StatusOK {
		return errors.New("watch endpoint returned non-200 status")
	}
	// 连接建立成功后，先发送一条 connected 事件。
	select {
	case <-ctx.Done():
		return ctx.Err()
	case events <- ConnectionEvent{Connected: true}:
	}
	// 使用 scanner 持续读取服务端心跳，直到 EOF 或上下文取消。
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	// 优先返回 scanner 的读取错误，否则返回 EOF 触发上层重连。
	if err := scanner.Err(); err != nil {
		return err
	}
	return errors.New("watch stream closed")
}
