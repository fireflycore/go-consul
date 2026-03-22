package registry

import micro "github.com/fireflycore/go-micro/registry"

// EventType 表示发现快照差异事件类型。
type EventType int

const (
	// EventAdd 表示新增实例事件。
	EventAdd EventType = iota
	// EventUpdate 表示实例变更事件。
	EventUpdate
	// EventDelete 表示实例删除事件。
	EventDelete
)

// ServiceEvent 是发现层事件载体。
type ServiceEvent struct {
	// Type 表示事件类型。
	Type    EventType
	// Service 表示发生变化的节点对象。
	Service *micro.ServiceNode
}
