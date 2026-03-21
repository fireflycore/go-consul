package registry

import micro "github.com/fireflycore/go-micro/registry"

type EventType int

const (
	EventAdd EventType = iota
	EventUpdate
	EventDelete
)

type ServiceEvent struct {
	Type    EventType
	Service *micro.ServiceNode
}
