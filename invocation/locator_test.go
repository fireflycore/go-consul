package invocation

import (
	"context"
	"testing"
	"time"

	microInvocation "github.com/fireflycore/go-micro/invocation"
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
		config: &Config{
			Namespace: "default",
			TargetOptions: microInvocation.TargetOptions{
				DefaultPort:    9000,
				ClusterDomain:  microInvocation.DefaultClusterDomain,
				ResolverScheme: microInvocation.DefaultResolverScheme,
			},
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
	locator := &Locator{
		health: fakeHealthService{
			rows: []*api.ServiceEntry{
				{
					Service: &api.AgentService{
						Service: "auth",
						Address: "127.0.0.1",
						Port:    9000,
						Meta: map[string]string{
							"namespace":       "default",
							"env":             "dev",
							"app_id":          "auth",
							"instance_id":     "i-1",
							"version":         "v1",
							"kernel_language": "golang",
							"kernel_version":  "1.3.4",
						},
					},
				},
			},
		},
		config: &Config{
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

func TestDecodeEndpointReadsGenericServiceMetadata(t *testing.T) {
	endpoint, ok := decodeEndpoint(&api.ServiceEntry{
		Service: &api.AgentService{
			Service: "auth",
			Address: "10.0.0.8",
			Port:    9000,
			Meta: map[string]string{
				"namespace":       "default",
				"env":             "prod",
				"app_id":          "auth",
				"instance_id":     "i-2",
				"version":         "v2",
				"weight":          "120",
				"kernel_language": "golang",
			},
		},
	}, microInvocation.ServiceRef{
		Service:   "auth",
		Namespace: "default",
		Env:       "prod",
	})
	if !ok {
		t.Fatalf("expected endpoint to be decoded")
	}
	if endpoint.Weight != 120 {
		t.Fatalf("unexpected weight: %d", endpoint.Weight)
	}
	if endpoint.Meta["app_id"] != "auth" {
		t.Fatalf("unexpected app_id: %s", endpoint.Meta["app_id"])
	}
	if endpoint.Meta["instance_id"] != "i-2" {
		t.Fatalf("unexpected instance_id: %s", endpoint.Meta["instance_id"])
	}
	if endpoint.Meta["kernel_language"] != "golang" {
		t.Fatalf("unexpected kernel_language: %s", endpoint.Meta["kernel_language"])
	}
}
