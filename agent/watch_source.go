package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	// ErrWatchURLRequired 表示业务侧未提供本地 `/watch` 地址。
	ErrWatchURLRequired = errors.New("watch url is required")
	// ErrWatchStreamClosed 表示当前 watch 流已由服务端关闭，调用方应进入下一轮重连。
	ErrWatchStreamClosed = errors.New("watch stream closed")
	// watchEventPayloadPool 复用结构化 SSE 载荷对象，降低高频 JSON 帧下的临时对象分配。
	watchEventPayloadPool = sync.Pool{
		New: func() any {
			return new(watchEventPayload)
		},
	}
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
	// 空错误对象统一返回空字符串，避免日志里出现 `<nil>` 噪音。
	if e == nil {
		return ""
	}
	// 如果服务端返回了完整状态文本，则优先保留原始状态文本。
	if strings.TrimSpace(e.Status) != "" {
		return fmt.Sprintf("watch endpoint returned non-200 status: %s", e.Status)
	}
	// 否则回退到仅输出状态码的错误文本。
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
	// 空错误对象统一返回空字符串。
	if e == nil {
		return ""
	}
	// 如果当前坏帧携带了 SSE 事件 ID，则一并带上，便于排障定位。
	if strings.TrimSpace(e.EventId) != "" {
		return fmt.Sprintf("watch event parse failed: type=%s id=%s: %v", e.EventType, e.EventId, e.Err)
	}
	// 否则仅输出事件类型和底层解析错误。
	return fmt.Sprintf("watch event parse failed: type=%s: %v", e.EventType, e.Err)
}

// Unwrap 返回底层解析错误。
func (e *WatchEventParseError) Unwrap() error {
	// 空错误对象时没有可展开的底层错误。
	if e == nil {
		return nil
	}
	// 返回底层 JSON 解析错误，便于 errors.Is / errors.As 继续匹配。
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
		// 保存清洗后的 watch 地址，供后续每轮重连直接复用。
		watchURL: cleanWatchURL,
		client: &http.Client{
			// SSE 连接需要持续阻塞读取，因此这里不设置整体超时。
			Timeout: 0,
		},
		// 保存断连后的固定退避间隔。
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
		// 复用一个定时器，避免在重连循环里反复创建 time.After 对象。
		timer := time.NewTimer(0)
		// 首轮不等待，因此立即清掉第一次触发。
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		// 协程退出前释放定时器资源。
		defer timer.Stop()
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
						// 如果在派发断连事件前上下文结束，则直接退出即可。
						return
					case events <- ConnectionEvent{Type: ConnectionEventTypeDisconnected, Connected: false, Err: err}:
						// 把本轮 watch 失败原因包装成 disconnected 事件交给上游处理。
					}
				}
			}
			// 在后续一次重连前等待固定退避，避免打满本地 agent。
			timer.Reset(s.reconnectInterval)
			select {
			case <-ctx.Done():
				// 等待退避期间如果上下文结束，则不再进入下一轮重连。
				return
			case <-timer.C:
				// 退避结束后进入下一轮 watch 连接尝试。
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
	// URL 非法或请求构造失败时，直接返回错误给上层。
	if err != nil {
		return err
	}
	// 明确告诉服务端当前连接期望接收 SSE。
	request.Header.Set("Accept", "text/event-stream")
	// 发起连接请求。
	response, err := s.client.Do(request)
	// 建连失败时直接返回底层网络错误。
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
	// 使用按行读取而不是 Scanner，避免单帧超过默认 64K 时被截断。
	reader := bufio.NewReader(response.Body)
	currentEventName := ""
	currentEventID := ""
	dataLines := make([]string, 0, 1)
	for {
		// 逐行读取服务端输出的 SSE 帧内容。
		line, err := reader.ReadString('\n')
		// 如果读取到换行，则去掉尾部换行，保留该行实际内容。
		line = strings.TrimSuffix(line, "\n")
		// 如果服务端使用 CRLF，则进一步去掉末尾的回车。
		line = strings.TrimSuffix(line, "\r")
		// 如果上层上下文已取消，则停止继续读取当前流。
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// 空行表示当前 SSE 帧结束，尝试把累计内容派发成连接事件。
		if line == "" {
			if emitErr := emitWatchEvent(ctx, events, currentEventName, currentEventID, dataLines); emitErr != nil {
				return emitErr
			}
			currentEventName = ""
			currentEventID = ""
			dataLines = dataLines[:0]
			// 如果空行同时伴随 EOF，说明流已经自然结束，不应继续空转读取。
			if errors.Is(err, io.EOF) {
				break
			}
			// 除 EOF 外的读取错误仍然需要立即返回。
			if err != nil {
				return err
			}
			continue
		}
		// 注释行只用于保活，不单独派发事件。
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		// 不符合 `field:value` 结构的行直接忽略，保持解析宽容。
		if !ok {
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			continue
		}
		// 按 SSE 规则仅移除冒号后的一个可选前导空格，保留其余内容。
		value = trimSSEFieldValue(value)
		switch strings.TrimSpace(field) {
		case "event":
			// `event:` 定义当前帧的事件类型。
			currentEventName = value
		case "data":
			// `data:` 允许多行，按原始顺序累计保存。
			dataLines = append(dataLines, value)
		case "id":
			// `id:` 保存当前帧的 SSE 事件 ID。
			currentEventID = value
		}
		// 如果这一轮已经读到 EOF，则说明当前行是最后一行，后续不再继续阻塞读取。
		if errors.Is(err, io.EOF) {
			break
		}
		// 除 EOF 外的读取错误应立即返回。
		if err != nil {
			return err
		}
	}
	// 如果流在最后一帧后没有额外空行，也尽量把最后一帧派发出去。
	if err := emitWatchEvent(ctx, events, currentEventName, currentEventID, dataLines); err != nil {
		return err
	}
	// EOF 视为当前流自然结束，交给上层统一转成断连与重连。
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
	payloadRaw := joinDataLines(dataLines)
	// 先基于事件头部构造最小事件对象，后续再按 payload 补全字段。
	event := ConnectionEvent{
		Type:    strings.TrimSpace(eventName),
		EventId: strings.TrimSpace(eventId),
	}
	// 优先按当前结构化 JSON 协议解析；解析失败时继续走兼容逻辑，不直接报错。
	if strings.TrimSpace(payloadRaw) != "" {
		// 从对象池里取一个 payload 结构，减少高频 JSON 帧下的临时分配。
		payload := watchEventPayloadPool.Get().(*watchEventPayload)
		// 每次复用前先清零，避免把上一帧数据泄漏到当前帧。
		*payload = watchEventPayload{}
		if err := json.Unmarshal([]byte(payloadRaw), payload); err == nil {
			// 结构化解析成功后，把 payload 字段映射到统一事件对象。
			event.Message = payload.Message
			event.Service = payload.Service
			event.Status = payload.Status
			event.LifecycleState = payload.LifecycleState
			event.Ready = payload.Ready
			event.GeneratedAt = payload.GeneratedAt
			if strings.TrimSpace(payload.Event) != "" {
				// 如果 payload 自己也声明了事件类型，则以 payload 为准。
				event.Type = strings.TrimSpace(payload.Event)
			}
		} else {
			// 解析失败时先归还对象池里的 payload，避免泄漏。
			watchEventPayloadPool.Put(payload)
			if looksLikeJSON(payloadRaw) {
				// 看起来像 JSON 的坏帧视为协议层错误，直接向上返回。
				return &WatchEventParseError{
					EventType: strings.TrimSpace(event.Type),
					EventId:   strings.TrimSpace(event.EventId),
					Payload:   payloadRaw,
					Err:       err,
				}
			}
			// 否则按旧协议把原始 data 文本当成 message 处理。
			event.Message = strings.TrimSpace(payloadRaw)
			// 已经提前归还对象池，这里把本地引用清空，避免后面重复 Put。
			payload = nil
		}
		if payload != nil {
			// 结构化解析成功时，在字段拷贝完成后归还 payload 到对象池。
			watchEventPayloadPool.Put(payload)
		}
	}
	switch event.Type {
	case ConnectionEventTypeConnected:
		// connected 明确表示当前已建立连接。
		event.Connected = true
	case ConnectionEventTypeHeartbeat:
		// heartbeat 只作为活性事件，不改写 connected 语义。
	default:
		// 未识别事件类型时保持向后兼容：老版本 `connected` 帧仍已在上面识别，其余帧默认忽略。
		if event.Type == "message" && strings.EqualFold(strings.TrimSpace(event.Message), "ok") {
			// 兼容旧 sidecar 的 `data: ok` 形式，把它映射成 connected。
			event.Type = ConnectionEventTypeConnected
			event.Connected = true
		} else {
			// 其余未知事件直接忽略，不向业务侧派发噪音事件。
			return nil
		}
	}
	select {
	case <-ctx.Done():
		// 派发前如果上下文已结束，则返回取消错误。
		return ctx.Err()
	case events <- event:
		// 成功把当前事件投递给上游运行器。
		return nil
	}
}

// looksLikeJSON 通过首字符做一个最小判断，决定是否应该把坏帧视为结构化载荷解析失败。
func looksLikeJSON(raw string) bool {
	// 先去掉前后空白，避免前导空格干扰首字符判断。
	trimmed := strings.TrimSpace(raw)
	// 只做最小启发式判断：对象或数组都视为 JSON 载荷。
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

// trimSSEFieldValue 按 SSE 规则仅移除冒号后的一个可选空格。
func trimSSEFieldValue(value string) string {
	// SSE 只约定可选去掉一个前导空格，不应吞掉更多内容。
	if strings.HasPrefix(value, " ") {
		return value[1:]
	}
	// 没有可裁剪空格时直接返回原值。
	return value
}

// joinDataLines 按 SSE 约定把多行 data 拼回原始 payload，同时减少单行场景的额外分配。
func joinDataLines(dataLines []string) string {
	switch len(dataLines) {
	case 0:
		// 没有 data 行时返回空字符串。
		return ""
	case 1:
		// 单行 data 直接返回原始内容，避免额外拼接分配。
		return dataLines[0]
	default:
		// 多行 data 按 SSE 规则用换行符合并。
		return strings.Join(dataLines, "\n")
	}
}
