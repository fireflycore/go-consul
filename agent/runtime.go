package agent

import (
	"context"
	"errors"
)

// ConnectionEvent 描述本机 agent 连接状态变化事件。
type ConnectionEvent struct {
	// Connected 表示当前事件是否意味着连接已建立。
	Connected bool
	// Err 保存连接断开或处理失败时的上下文错误。
	Err error
}

// EventSource 抽象本地 agent 连接事件来源。
type EventSource interface {
	// Subscribe 返回一个持续输出连接事件的只读通道。
	Subscribe(ctx context.Context) (<-chan ConnectionEvent, error)
}

// ErrorHandler 用于统一处理运行时中的非致命错误。
type ErrorHandler func(context.Context, error)

// Runner 负责把连接事件转换成 register 重放动作。
type Runner struct {
	// source 提供本机 agent 的连接事件流。
	source EventSource
	// controller 负责把连接恢复事件转换成 register / drain / deregister 逻辑。
	controller *Controller
	// onError 用于统一处理订阅、注册重放等阶段的错误。
	onError ErrorHandler
}

// NewRunner 创建一个新的连接事件驱动运行器。
func NewRunner(source EventSource, controller *Controller, onError ErrorHandler) (*Runner, error) {
	// 对关键依赖做非空校验，避免运行期出现 nil 调用。
	switch {
	case source == nil:
		return nil, errors.New("event source is required")
	case controller == nil:
		return nil, errors.New("controller is required")
	default:
		// 依赖齐备时返回可直接运行的事件驱动器。
		return &Runner{
			source:     source,
			controller: controller,
			onError:    onError,
		}, nil
	}
}

// Run 持续消费连接事件，并在连接恢复后自动重放 register。
func (r *Runner) Run(ctx context.Context) error {
	// 先向事件源订阅连接事件流。
	events, err := r.source.Subscribe(ctx)
	if err != nil {
		return err
	}
	// 持续消费连接事件，直到上下文取消或事件源关闭。
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-events:
			if !ok {
				return nil
			}
			// 连接断开时先更新控制器状态，再透传错误上下文。
			if !event.Connected {
				r.controller.OnDisconnected()
				if event.Err != nil && r.onError != nil {
					r.onError(ctx, event.Err)
				}
				continue
			}
			// 连接建立或恢复时，立即重放 register 以重新接管业务实例。
			if err := r.controller.OnConnected(ctx); err != nil {
				// 重放失败时，把当前状态回退为 disconnected，避免误认为已接管成功。
				r.controller.OnDisconnected()
				if r.onError != nil {
					r.onError(ctx, err)
				}
			}
		}
	}
}
