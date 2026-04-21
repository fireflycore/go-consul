package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrWatchURLRequired 表示业务侧未提供本地 `/watch` 地址。
	ErrWatchURLRequired = errors.New("watch url is required")
	// ErrWatchStreamClosed 表示当前 watch 流已由服务端关闭，调用方应进入下一轮重连。
	ErrWatchStreamClosed = errors.New("watch stream closed")
)

// WatchHTTPStatusError 表示 `/watch` 接口返回了非 200 状态码。
type WatchHTTPStatusError struct {
	// StatusCode 表示 HTTP 状态码。
	StatusCode int
	// Status 表示原始 HTTP 状态文本。
	Status string
}

// Error 返回可读错误文本。
func (e *WatchHTTPStatusError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Status) != "" {
		return fmt.Sprintf("watch endpoint returned non-200 status: %s", e.Status)
	}
	return fmt.Sprintf("watch endpoint returned non-200 status: %d", e.StatusCode)
}

// WatchEventParseError 表示 SSE 事件帧存在结构化载荷解析错误。
type WatchEventParseError struct {
	// EventType 表示解析失败的事件类型。
	EventType string
	// EventId 表示服务端 SSE 事件 ID。
	EventId string
	// Payload 表示原始 data 内容，便于调试坏帧。
	Payload string
	// Err 表示底层 JSON 解析错误。
	Err error
}

// Error 返回可读错误文本。
func (e *WatchEventParseError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.EventId) != "" {
		return fmt.Sprintf("watch event parse failed: type=%s id=%s: %v", e.EventType, e.EventId, e.Err)
	}
	return fmt.Sprintf("watch event parse failed: type=%s: %v", e.EventType, e.Err)
}

// Unwrap 返回底层解析错误。
func (e *WatchEventParseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// watchEventPayload 描述 sidecar-agent `/watch` 输出的结构化 SSE 载荷。
type watchEventPayload struct {
	// Event 表示服务端事件名，例如 connected、heartbeat。
	Event string `json:"event"`
	// Message 表示简短说明文本。
	Message string `json:"message,omitempty"`
	// Service 表示发出事件的服务名。
	Service string `json:"service,omitempty"`
	// Status 表示 sidecar 当前运行态摘要。
	Status string `json:"status,omitempty"`
	// LifecycleState 表示 sidecar 当前生命周期阶段。
	LifecycleState string `json:"lifecycle_state,omitempty"`
	// Ready 表示 sidecar 当前主链是否 ready。
	Ready *bool `json:"ready,omitempty"`
	// GeneratedAt 表示服务端生成事件的时间。
	GeneratedAt *time.Time `json:"generated_at,omitempty"`
}

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
		return nil, ErrWatchURLRequired
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
					case events <- ConnectionEvent{Type: ConnectionEventTypeDisconnected, Connected: false, Err: err}:
					}
				}
			}
			// 在后续一次重连前等待固定退避，避免打满本地 agent。
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
		return &WatchHTTPStatusError{
			StatusCode: response.StatusCode,
			Status:     response.Status,
		}
	}
	// 连接建立成功后，先发送一条 connected 事件。
	// 使用 scanner 持续读取服务端 SSE 帧，直到 EOF 或上下文取消。
	scanner := bufio.NewScanner(response.Body)
	currentEventName := ""
	currentEventID := ""
	dataLines := make([]string, 0, 1)
	for scanner.Scan() {
		line := scanner.Text()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// 空行表示当前 SSE 帧结束，尝试把累计内容派发成连接事件。
		if line == "" {
			if err := emitWatchEvent(ctx, events, currentEventName, currentEventID, dataLines); err != nil {
				return err
			}
			currentEventName = ""
			currentEventID = ""
			dataLines = dataLines[:0]
			continue
		}
		// 注释行只用于保活，不单独派发事件。
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimLeft(value, " ")
		switch strings.TrimSpace(field) {
		case "event":
			currentEventName = value
		case "data":
			dataLines = append(dataLines, value)
		case "id":
			currentEventID = value
		}
	}
	// 如果流在最后一帧后没有额外空行，也尽量把最后一帧派发出去。
	if err := emitWatchEvent(ctx, events, currentEventName, currentEventID, dataLines); err != nil {
		return err
	}
	// 优先返回 scanner 的读取错误，否则返回 EOF 触发上层重连。
	if err := scanner.Err(); err != nil {
		return err
	}
	return ErrWatchStreamClosed
}

// emitWatchEvent 把一帧 SSE 内容转换成业务侧连接事件。
func emitWatchEvent(ctx context.Context, events chan<- ConnectionEvent, eventName, eventId string, dataLines []string) error {
	// 空帧直接忽略，避免纯 heartbeat 注释或空白块触发无意义事件。
	if strings.TrimSpace(eventName) == "" && len(dataLines) == 0 {
		return nil
	}
	// 默认按 SSE 约定回退到 message；当前协议未使用时仍可兼容旧服务端。
	if strings.TrimSpace(eventName) == "" {
		eventName = "message"
	}
	payloadRaw := strings.Join(dataLines, "\n")
	event := ConnectionEvent{
		Type:    strings.TrimSpace(eventName),
		EventId: strings.TrimSpace(eventId),
	}
	// 优先按当前结构化 JSON 协议解析；解析失败时继续走兼容逻辑，不直接报错。
	if strings.TrimSpace(payloadRaw) != "" {
		var payload watchEventPayload
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err == nil {
			event.Message = payload.Message
			event.Service = payload.Service
			event.Status = payload.Status
			event.LifecycleState = payload.LifecycleState
			event.Ready = payload.Ready
			event.GeneratedAt = payload.GeneratedAt
			if strings.TrimSpace(payload.Event) != "" {
				event.Type = strings.TrimSpace(payload.Event)
			}
		} else {
			if looksLikeJSON(payloadRaw) {
				return &WatchEventParseError{
					EventType: strings.TrimSpace(event.Type),
					EventId:   strings.TrimSpace(event.EventId),
					Payload:   payloadRaw,
					Err:       err,
				}
			}
			event.Message = strings.TrimSpace(payloadRaw)
		}
	}
	switch event.Type {
	case ConnectionEventTypeConnected:
		event.Connected = true
	case ConnectionEventTypeHeartbeat:
		// heartbeat 只作为活性事件，不改写 connected 语义。
	default:
		// 未识别事件类型时保持向后兼容：老版本 `connected` 帧仍已在上面识别，其余帧默认忽略。
		if event.Type == "message" && strings.EqualFold(strings.TrimSpace(event.Message), "ok") {
			event.Type = ConnectionEventTypeConnected
			event.Connected = true
		} else {
			return nil
		}
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case events <- event:
		return nil
	}
}

// looksLikeJSON 通过首字符做一个最小判断，决定是否应该把坏帧视为结构化载荷解析失败。
func looksLikeJSON(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}
