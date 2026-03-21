package registry

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	micro "github.com/fireflycore/go-micro/registry"
	"github.com/hashicorp/consul/api"
)

type DiscoverInstance struct {
	mu sync.RWMutex

	client *api.Client
	meta   *micro.Meta
	conf   *micro.ServiceConf

	method  micro.ServiceMethod
	service micro.ServiceDiscover

	stopCh chan struct{}
	once   sync.Once
}

func NewDiscover(client *api.Client, meta *micro.Meta, conf *micro.ServiceConf) (*DiscoverInstance, error) {
	if client == nil {
		return nil, fmt.Errorf(micro.ErrClientIsNilFormat, "consul")
	}
	if meta == nil {
		return nil, micro.ErrServiceMetaIsNil
	}
	if conf == nil {
		return nil, micro.ErrServiceConfIsNil
	}
	conf.Bootstrap()

	return &DiscoverInstance{
		client:  client,
		meta:    meta,
		conf:    conf,
		method:  make(micro.ServiceMethod),
		service: make(micro.ServiceDiscover),
		stopCh:  make(chan struct{}),
	}, nil
}

func (s *DiscoverInstance) GetService(method string) ([]*micro.ServiceNode, string, error) {
	s.mu.RLock()
	appID, ok := s.method[method]
	if !ok {
		s.mu.RUnlock()
		return nil, "", micro.ErrServiceMethodNotExists
	}
	nodes, ok := s.service[appID]
	if !ok {
		s.mu.RUnlock()
		return nil, appID, micro.ErrServiceNodeNotExists
	}
	out := append([]*micro.ServiceNode(nil), nodes...)
	s.mu.RUnlock()
	return out, appID, nil
}

func (s *DiscoverInstance) Watcher() {
	_ = s.refreshAndDispatch()
	s.watchLoop()
}

func (s *DiscoverInstance) Unwatch() {
	s.once.Do(func() { close(s.stopCh) })
}

func (s *DiscoverInstance) watchLoop() {
	var waitIndex uint64
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		_, meta, err := s.client.Catalog().Services(&api.QueryOptions{
			WaitIndex: waitIndex,
			WaitTime:  60 * time.Second,
		})
		if err != nil {
			timer := time.NewTimer(time.Second)
			select {
			case <-s.stopCh:
				timer.Stop()
				return
			case <-timer.C:
				continue
			}
		}

		if meta != nil && meta.LastIndex > waitIndex {
			waitIndex = meta.LastIndex
		}

		_ = s.refreshAndDispatch()
	}
}

func (s *DiscoverInstance) refreshAndDispatch() error {
	nextService, err := s.fetchHealthyServices()
	if err != nil {
		return err
	}

	s.mu.Lock()
	before := s.service
	s.service = nextService
	s.refreshMethodsLocked()
	_ = buildEvents(before, nextService)
	s.mu.Unlock()
	return nil
}

func (s *DiscoverInstance) fetchHealthyServices() (micro.ServiceDiscover, error) {
	services, _, err := s.client.Catalog().Services(&api.QueryOptions{})
	if err != nil {
		return nil, err
	}

	dst := make(micro.ServiceDiscover)
	for appID := range services {
		nodes, err := s.fetchServiceNodes(appID)
		if err != nil {
			continue
		}
		if len(nodes) == 0 {
			continue
		}
		dst[appID] = nodes
	}
	return dst, nil
}

func (s *DiscoverInstance) fetchServiceNodes(appID string) ([]*micro.ServiceNode, error) {
	rows, _, err := s.client.Health().Service(appID, "", true, &api.QueryOptions{})
	if err != nil {
		return nil, err
	}

	nodes := make([]*micro.ServiceNode, 0, len(rows))
	for _, row := range rows {
		node := decodeServiceNode(row)
		if node == nil || node.Meta == nil || node.Meta.Env != s.meta.Env {
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (s *DiscoverInstance) refreshMethodsLocked() {
	next := make(micro.ServiceMethod)
	for appID, nodes := range s.service {
		for _, node := range nodes {
			if node == nil {
				continue
			}
			for method := range node.Methods {
				next[method] = appID
			}
		}
	}
	s.method = next
}

func decodeServiceNode(row *api.ServiceEntry) *micro.ServiceNode {
	if row == nil || row.Service == nil {
		return nil
	}

	if row.Service.Meta != nil {
		if raw, ok := row.Service.Meta["node"]; ok && raw != "" {
			var node micro.ServiceNode
			if err := json.Unmarshal([]byte(raw), &node); err == nil {
				return &node
			}
		}
	}

	node := &micro.ServiceNode{
		Methods: map[string]bool{},
		Meta: &micro.Meta{
			AppId:   row.Service.Service,
			Env:     row.Service.Meta["env"],
			Version: row.Service.Meta["version"],
		},
		Network: &micro.Network{
			SN:       row.Service.Meta["network_sn"],
			Internal: fmt.Sprintf("%s:%d", row.Service.Address, row.Service.Port),
			External: row.Service.Meta["network_external"],
		},
		Kernel: &micro.Kernel{
			Language: row.Service.Meta["kernel_language"],
			Version:  row.Service.Meta["kernel_version"],
		},
		RunDate: row.Service.Meta["run_date"],
	}

	if weightRaw := row.Service.Meta["weight"]; weightRaw != "" {
		if weight, err := strconv.Atoi(weightRaw); err == nil {
			node.Weight = weight
		}
	}

	if methodsRaw := row.Service.Meta["methods"]; methodsRaw != "" {
		_ = json.Unmarshal([]byte(methodsRaw), &node.Methods)
	}
	if len(node.Methods) == 0 && len(row.Service.Tags) != 0 {
		for _, tag := range row.Service.Tags {
			if strings.HasPrefix(tag, "method:") {
				node.Methods[strings.TrimPrefix(tag, "method:")] = true
			}
		}
	}

	return node
}

func buildEvents(before, after micro.ServiceDiscover) []ServiceEvent {
	events := make([]ServiceEvent, 0, len(before)+len(after))
	for appID, nodes := range after {
		prev, ok := before[appID]
		if !ok {
			for _, node := range nodes {
				events = append(events, ServiceEvent{Type: EventAdd, Service: node})
			}
			continue
		}
		if !sameNodes(prev, nodes) {
			for _, node := range nodes {
				events = append(events, ServiceEvent{Type: EventUpdate, Service: node})
			}
		}
	}

	for appID, nodes := range before {
		if _, ok := after[appID]; ok {
			continue
		}
		for _, node := range nodes {
			events = append(events, ServiceEvent{Type: EventDelete, Service: node})
		}
	}

	return events
}

func sameNodes(left, right []*micro.ServiceNode) bool {
	if len(left) != len(right) {
		return false
	}
	lm := make(map[string]struct{}, len(left))
	for _, n := range left {
		if n == nil || n.Meta == nil || n.Network == nil {
			continue
		}
		lm[n.Meta.AppId+"|"+n.Network.Internal] = struct{}{}
	}
	for _, n := range right {
		if n == nil || n.Meta == nil || n.Network == nil {
			continue
		}
		if _, ok := lm[n.Meta.AppId+"|"+n.Network.Internal]; !ok {
			return false
		}
	}
	return true
}
