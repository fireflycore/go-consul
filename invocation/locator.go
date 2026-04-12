package invocation

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	microInvocation "github.com/fireflycore/go-micro/invocation"
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
	config *Config

	mu      sync.Mutex
	cache   map[string]cachedEndpoints
	cursor  map[string]uint64
	nowFunc func() time.Time
}

// NewLocator 创建 Consul invocation 定位器。
func NewLocator(client *api.Client, config *Config) (*Locator, error) {
	if client == nil {
		return nil, errors.New("consul client is nil")
	}
	if config == nil {
		config = &Config{}
	}
	config.Bootstrap()

	return &Locator{
		health:  client.Health(),
		config:  config,
		cache:   make(map[string]cachedEndpoints),
		cursor:  make(map[string]uint64),
		nowFunc: time.Now,
	}, nil
}

// Resolve 根据 ServiceRef 解析最终 Target。
func (l *Locator) Resolve(ctx context.Context, ref microInvocation.ServiceRef) (microInvocation.Target, error) {
	ref = l.normalizeRef(ref)
	if l.config.PreferServiceDNS {
		return microInvocation.StaticLocator{
			Options: l.config.TargetOptions,
		}.Resolve(ctx, ref)
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
		ref.Namespace = l.config.Namespace
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
		expiresAt: l.nowFunc().Add(l.config.CacheTTL),
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
	if row == nil || row.Service == nil {
		return microInvocation.ServiceEndpoint{}, false
	}
	meta := copyMeta(row.Service.Meta)
	if meta["namespace"] != ref.NamespaceName() {
		return microInvocation.ServiceEndpoint{}, false
	}
	if strings.TrimSpace(ref.Env) != "" && meta["env"] != strings.TrimSpace(ref.Env) {
		return microInvocation.ServiceEndpoint{}, false
	}

	address := row.Service.Address
	if address == "" && row.Node != nil {
		address = row.Node.Address
	}
	if address == "" || row.Service.Port == 0 {
		return microInvocation.ServiceEndpoint{}, false
	}

	endpoint := microInvocation.ServiceEndpoint{
		Address: net.JoinHostPort(address, strconv.Itoa(row.Service.Port)),
		Weight:  parseWeight(meta["weight"]),
		Healthy: true,
		Meta:    meta,
	}

	return endpoint, true
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

func copyMeta(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
