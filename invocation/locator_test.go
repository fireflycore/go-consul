package invocation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	microInvocation "github.com/fireflycore/go-micro/invocation"
	microRegistry "github.com/fireflycore/go-micro/registry"
	"github.com/hashicorp/consul/api"
)

type fakeHealthService struct {
	rows []*api.ServiceEntry
	err  error
}

func (f fakeHealthService) Service(service, tag string, passingOnly bool, q *api.QueryOptions) ([]*api.ServiceEntry, *api.QueryMeta, error) {
	return f.rows, &api.QueryMeta{}, f.err
}

func TestLocatorResolvePreferServiceDNS(t *testing.T) {
	locator := &Locator{
		health: fakeHealthService{},
		conf: Conf{
			Namespace:        "default",
			DefaultPort:      9000,
			ClusterDomain:    microInvocation.DefaultClusterDomain,
			ResolverScheme:   microInvocation.DefaultResolverScheme,
			PreferServiceDNS: true,
			CacheTTL:         DefaultCacheTTL,
		},
		cache:   map[string]cachedEndpoints{},
		cursor:  map[string]uint64{},
		nowFunc: func() time.Time { return time.Unix(0, 0) },
	}

	target, err := locator.Resolve(context.Background(), microInvocation.ServiceRef{
		Service:   "auth",
		Namespace: "default",
		Env:       "dev",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if target.GRPCTarget() != "dns:///auth.default.svc.cluster.local:9000" {
		t.Fatalf("unexpected target: %s", target.GRPCTarget())
	}
}

func TestLocatorResolveReturnsEndpointTarget(t *testing.T) {
	raw, err := json.Marshal(&microRegistry.ServiceNode{
		Weight: 100,
		Meta: &microRegistry.ServiceMeta{
			AppId:      "auth",
			InstanceId: "i-1",
			Env:        "dev",
		},
		Network: &microRegistry.Network{
			Internal: "127.0.0.1:9000",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	locator := &Locator{
		health: fakeHealthService{
			rows: []*api.ServiceEntry{
				{
					Service: &api.AgentService{
						Service: "auth",
						Port:    9000,
						Meta: map[string]string{
							"namespace": "default",
							"env":       "dev",
							"node":      string(raw),
						},
					},
				},
			},
		},
		conf: Conf{
			Namespace: "default",
			CacheTTL:  DefaultCacheTTL,
		},
		cache:   map[string]cachedEndpoints{},
		cursor:  map[string]uint64{},
		nowFunc: time.Now,
	}

	target, err := locator.Resolve(context.Background(), microInvocation.ServiceRef{
		Service:   "auth",
		Namespace: "default",
		Env:       "dev",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if target.Host != "127.0.0.1" {
		t.Fatalf("unexpected host: %s", target.Host)
	}
	if target.Port != 9000 {
		t.Fatalf("unexpected port: %d", target.Port)
	}
}
