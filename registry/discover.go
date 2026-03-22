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

// DiscoverInstance 封装 Consul 网关发现逻辑。
type DiscoverInstance struct {
	// mu 保护 method 与 service 两级索引。
	mu sync.RWMutex

	// client 是 Consul API 客户端。
	client *api.Client
	// meta 是当前网关环境元数据。
	meta *micro.Meta
	// conf 是发现配置。
	conf *ServiceConf

	// method 保存 method -> appId 的映射。
	method micro.ServiceMethod
	// service 保存 appId -> service nodes 的映射。
	service micro.ServiceDiscover

	// stopCh 用于终止 watch 循环。
	stopCh chan struct{}
	// once 确保 stopCh 只关闭一次。
	once sync.Once

	// watchEventCallback 用于向外透传服务变更事件。
	watchEventCallback micro.WatchEventFunc
}

// NewDiscover 创建发现器并返回统一发现接口。
func NewDiscover(client *api.Client, meta *micro.Meta, conf *ServiceConf) (micro.Discovery, error) {
	// 客户端不能为空。
	if client == nil {
		return nil, fmt.Errorf(micro.ErrClientIsNilFormat, "consul")
	}
	// 元数据不能为空。
	if meta == nil {
		return nil, micro.ErrServiceMetaIsNil
	}
	// 配置不能为空。
	if conf == nil {
		return nil, micro.ErrServiceConfIsNil
	}
	// 补齐默认配置。
	conf.Bootstrap()

	// 初始化发现器对象及内存索引。
	return &DiscoverInstance{
		client:  client,
		meta:    meta,
		conf:    conf,
		method:  make(micro.ServiceMethod),
		service: make(micro.ServiceDiscover),
		stopCh:  make(chan struct{}),
	}, nil
}

// GetService 根据 method 查询服务节点列表。
func (s *DiscoverInstance) GetService(method string) ([]*micro.ServiceNode, string, error) {
	// 先查 method 索引得到 appId。
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
	// 返回副本，避免调用方改动内部切片。
	out := append([]*micro.ServiceNode(nil), nodes...)
	s.mu.RUnlock()
	return out, appID, nil
}

// Watcher 启动发现刷新与阻塞监听。
func (s *DiscoverInstance) Watcher() {
	// 先做一次全量刷新，确保初始可用。
	_ = s.refreshAndDispatch()
	// 进入阻塞监听循环。
	s.watchLoop()
}

// Unwatch 停止监听流程。
func (s *DiscoverInstance) Unwatch() {
	s.once.Do(func() { close(s.stopCh) })
}

// WatchEvent 注册服务变更回调。
func (s *DiscoverInstance) WatchEvent(callback micro.WatchEventFunc) {
	s.watchEventCallback = callback
}

// watchLoop 使用 Consul 阻塞查询监听服务目录变化。
func (s *DiscoverInstance) watchLoop() {
	// waitIndex 保存本轮阻塞查询起点。
	var waitIndex uint64
	for {
		// 支持外部停止信号。
		select {
		case <-s.stopCh:
			return
		default:
		}

		// Catalog.Services + WaitIndex 形成阻塞查询。
		_, meta, err := s.client.Catalog().Services(&api.QueryOptions{
			WaitIndex: waitIndex,
			WaitTime:  60 * time.Second,
		})
		if err != nil {
			// 查询失败后短暂退避，避免忙等。
			timer := time.NewTimer(time.Second)
			select {
			case <-s.stopCh:
				timer.Stop()
				return
			case <-timer.C:
				continue
			}
		}

		// 仅当 LastIndex 前进时推进 waitIndex。
		if meta != nil && meta.LastIndex > waitIndex {
			waitIndex = meta.LastIndex
		}

		// 每次变化后刷新本地缓存。
		_ = s.refreshAndDispatch()
	}
}

// refreshAndDispatch 全量刷新 appId->nodes 与 method->appId 索引。
func (s *DiscoverInstance) refreshAndDispatch() error {
	// 从 Consul 拉取全量健康节点。
	nextService, err := s.fetchHealthyServices()
	if err != nil {
		return err
	}

	// 原子替换索引。
	s.mu.Lock()
	before := s.service
	s.service = nextService
	s.refreshMethodsLocked()
	events := buildEvents(before, nextService)
	s.mu.Unlock()

	s.dispatchEvents(events)
	return nil
}

// fetchHealthyServices 抓取所有 appId 的健康实例。
func (s *DiscoverInstance) fetchHealthyServices() (micro.ServiceDiscover, error) {
	// 先取服务名列表。
	services, _, err := s.client.Catalog().Services(&api.QueryOptions{})
	if err != nil {
		return nil, err
	}

	// 逐服务拉取健康节点并写入目标映射。
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

// fetchServiceNodes 拉取指定 appId 的健康节点并做环境过滤。
func (s *DiscoverInstance) fetchServiceNodes(appID string) ([]*micro.ServiceNode, error) {
	// passingOnly=true 仅返回健康节点。
	rows, _, err := s.client.Health().Service(appID, "", true, &api.QueryOptions{})
	if err != nil {
		return nil, err
	}

	// 解码节点并过滤到当前环境。
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

// refreshMethodsLocked 基于 service 索引重建 method 索引。
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

// decodeServiceNode 把 Consul ServiceEntry 转回通用 ServiceNode。
func decodeServiceNode(row *api.ServiceEntry) *micro.ServiceNode {
	// 防御空指针。
	if row == nil || row.Service == nil {
		return nil
	}

	// 优先尝试从 node 元数据完整反序列化。
	if row.Service.Meta != nil {
		if raw, ok := row.Service.Meta["node"]; ok && raw != "" {
			var node micro.ServiceNode
			if err := json.Unmarshal([]byte(raw), &node); err == nil {
				return &node
			}
		}
	}

	// 回退路径：从离散字段拼装 ServiceNode。
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

	// 解析权重字段。
	if weightRaw := row.Service.Meta["weight"]; weightRaw != "" {
		if weight, err := strconv.Atoi(weightRaw); err == nil {
			node.Weight = weight
		}
	}

	// 优先解析 methods JSON。
	if methodsRaw := row.Service.Meta["methods"]; methodsRaw != "" {
		_ = json.Unmarshal([]byte(methodsRaw), &node.Methods)
	}
	// 若 methods 为空，则尝试兼容旧式 method:xxx tag。
	if len(node.Methods) == 0 && len(row.Service.Tags) != 0 {
		for _, tag := range row.Service.Tags {
			if strings.HasPrefix(tag, "method:") {
				node.Methods[strings.TrimPrefix(tag, "method:")] = true
			}
		}
	}

	return node
}

// buildEvents 对比前后快照，生成增删改事件集合。
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

// sameNodes 判断两组节点是否是同一集合。
func sameNodes(left, right []*micro.ServiceNode) bool {
	// 长度不同可直接判定不相同。
	if len(left) != len(right) {
		return false
	}
	// 构建左侧索引。
	lm := make(map[string]struct{}, len(left))
	for _, n := range left {
		if n == nil || n.Meta == nil || n.Network == nil {
			continue
		}
		lm[n.Meta.AppId+"|"+n.Network.Internal] = struct{}{}
	}
	// 校验右侧节点是否都在左侧索引中。
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

func (s *DiscoverInstance) dispatchEvents(events []ServiceEvent) {
	if s.watchEventCallback == nil || len(events) == 0 {
		return
	}
	for _, event := range events {
		raw := &micro.ServiceEvent{
			Type:    micro.EventType(event.Type),
			Service: event.Service,
		}
		go s.watchEventCallback(raw)
	}
}
