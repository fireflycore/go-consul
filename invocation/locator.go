package invocation

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	microInvocation "github.com/fireflycore/go-micro/invocation"
	microRegistry "github.com/fireflycore/go-micro/registry"
	"github.com/hashicorp/consul/api"
)

// healthService 抽象 Consul 健康查询能力，便于注入测试替身。
type healthService interface {
	Service(service, tag string, passingOnly bool, q *api.QueryOptions) ([]*api.ServiceEntry, *api.QueryMeta, error)
}

type cachedEndpoints struct {
	endpoints []microInvocation.ServiceEndpoint
	expiresAt time.Time
}

// Locator 是基于 Consul 的 invocation 定位器。
//
// 它与 etcd 轻量实现保持一致：
// - 若已具备稳定 service DNS，则优先返回 DNS target；
// - 否则读取 Consul 健康实例列表，在本地做轻量 endpoint 选择。
type Locator struct {
	health healthService
	conf   Conf

	mu      sync.Mutex
	cache   map[string]cachedEndpoints
	cursor  map[string]uint64
	nowFunc func() time.Time
}

// NewLocator 创建 Consul invocation 定位器。
func NewLocator(client *api.Client, conf *Conf) (*Locator, error) {
	if client == nil {
		return nil, fmt.Errorf(microRegistry.ErrClientIsNilFormat, "consul")
	}
	if conf == nil {
		conf = &Conf{}
	}
	conf.Bootstrap()

	return &Locator{
		health:  client.Health(),
		conf:    *conf,
		cache:   make(map[string]cachedEndpoints),
		cursor:  make(map[string]uint64),
		nowFunc: time.Now,
	}, nil
}

// Resolve 根据 ServiceRef 解析最终 Target。
func (l *Locator) Resolve(ctx context.Context, ref microInvocation.ServiceRef) (microInvocation.Target, error) {
	_ = ctx

	ref = l.normalizeRef(ref)
	if l.conf.PreferServiceDNS {
		return microInvocation.BuildTarget(ref, microInvocation.TargetOptions{
			DefaultPort:    l.conf.DefaultPort,
			ClusterDomain:  l.conf.ClusterDomain,
			ResolverScheme: l.conf.ResolverScheme,
		})
	}

	endpoints, err := l.loadEndpoints(ref)
	if err != nil {
		return microInvocation.Target{}, err
	}

	endpoint, err := l.pickEndpoint(ref, endpoints)
	if err != nil {
		return microInvocation.Target{}, err
	}

	host, port, err := splitAddress(endpoint.Address)
	if err != nil {
		return microInvocation.Target{}, err
	}

	return microInvocation.Target{
		Host: host,
		Port: port,
	}, nil
}

func (l *Locator) normalizeRef(ref microInvocation.ServiceRef) microInvocation.ServiceRef {
	if strings.TrimSpace(ref.Namespace) == "" {
		ref.Namespace = l.conf.Namespace
	}
	return ref
}

func (l *Locator) loadEndpoints(ref microInvocation.ServiceRef) ([]microInvocation.ServiceEndpoint, error) {
	key := l.cacheKey(ref)

	l.mu.Lock()
	if item, ok := l.cache[key]; ok && l.nowFunc().Before(item.expiresAt) {
		out := append([]microInvocation.ServiceEndpoint(nil), item.endpoints...)
		l.mu.Unlock()
		return out, nil
	}
	l.mu.Unlock()

	endpoints, err := l.fetchEndpoints(ref)
	if err != nil {
		return nil, err
	}

	l.mu.Lock()
	l.cache[key] = cachedEndpoints{
		endpoints: append([]microInvocation.ServiceEndpoint(nil), endpoints...),
		expiresAt: l.nowFunc().Add(l.conf.CacheTTL),
	}
	l.mu.Unlock()

	return endpoints, nil
}

func (l *Locator) fetchEndpoints(ref microInvocation.ServiceRef) ([]microInvocation.ServiceEndpoint, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}

	rows, _, err := l.health.Service(ref.ServiceName(), "", true, &api.QueryOptions{})
	if err != nil {
		return nil, err
	}

	endpoints := make([]microInvocation.ServiceEndpoint, 0, len(rows))
	for _, row := range rows {
		endpoint, ok := decodeEndpoint(row, ref)
		if !ok {
			continue
		}
		endpoints = append(endpoints, endpoint)
	}
	if len(endpoints) == 0 {
		return nil, microInvocation.ErrTargetHostEmpty
	}

	return endpoints, nil
}

func (l *Locator) pickEndpoint(ref microInvocation.ServiceRef, endpoints []microInvocation.ServiceEndpoint) (microInvocation.ServiceEndpoint, error) {
	if len(endpoints) == 0 {
		return microInvocation.ServiceEndpoint{}, microInvocation.ErrTargetHostEmpty
	}

	key := l.cacheKey(ref)

	l.mu.Lock()
	defer l.mu.Unlock()

	index := l.cursor[key] % uint64(len(endpoints))
	l.cursor[key]++
	return endpoints[index], nil
}

func (l *Locator) cacheKey(ref microInvocation.ServiceRef) string {
	return strings.Join([]string{ref.ServiceName(), ref.NamespaceName(), strings.TrimSpace(ref.Env)}, "|")
}

func decodeEndpoint(row *api.ServiceEntry, ref microInvocation.ServiceRef) (microInvocation.ServiceEndpoint, bool) {
	if row == nil || row.Service == nil || row.Service.Meta == nil {
		return microInvocation.ServiceEndpoint{}, false
	}
	if row.Service.Meta["namespace"] != ref.NamespaceName() {
		return microInvocation.ServiceEndpoint{}, false
	}
	if strings.TrimSpace(ref.Env) != "" && row.Service.Meta["env"] != strings.TrimSpace(ref.Env) {
		return microInvocation.ServiceEndpoint{}, false
	}

	var node microRegistry.ServiceNode
	if raw := row.Service.Meta["node"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &node); err == nil && node.Network != nil && node.Network.Internal != "" {
			return microInvocation.ServiceEndpoint{
				Address: node.Network.Internal,
				Weight:  node.Weight,
				Healthy: true,
				Meta: map[string]string{
					"app_id":      row.Service.Meta["app_id"],
					"instance_id": row.Service.Meta["instance_id"],
					"env":         row.Service.Meta["env"],
					"version":     row.Service.Meta["version"],
				},
			}, true
		}
	}

	address := row.Service.Address
	if address == "" {
		address = row.Node.Address
	}
	if address == "" || row.Service.Port == 0 {
		return microInvocation.ServiceEndpoint{}, false
	}

	return microInvocation.ServiceEndpoint{
		Address: net.JoinHostPort(address, strconv.Itoa(row.Service.Port)),
		Weight:  parseWeight(row.Service.Meta["weight"]),
		Healthy: true,
		Meta: map[string]string{
			"app_id":      row.Service.Meta["app_id"],
			"instance_id": row.Service.Meta["instance_id"],
			"env":         row.Service.Meta["env"],
			"version":     row.Service.Meta["version"],
		},
	}, true
}

func parseWeight(raw string) int {
	if raw == "" {
		return 0
	}
	weight, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return weight
}

func splitAddress(raw string) (string, uint16, error) {
	host, portRaw, err := net.SplitHostPort(raw)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		return "", 0, err
	}
	return host, uint16(port), nil
}
