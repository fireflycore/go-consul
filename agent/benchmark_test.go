package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/fireflycore/go-micro/app"
	"github.com/fireflycore/go-micro/kernel"
	"github.com/fireflycore/go-micro/service"
)

type benchmarkNoopClient struct{}

func (benchmarkNoopClient) Register(ctx context.Context, request *ServiceNode) error {
	return nil
}

func (benchmarkNoopClient) Drain(ctx context.Context, request DrainRequest) error {
	return nil
}

func (benchmarkNoopClient) Deregister(ctx context.Context, request DeregisterRequest) error {
	return nil
}

func benchmarkServiceOptions() *ServiceOptions {
	return &ServiceOptions{
		App: app.Config{
			Id:         "10001",
			Name:       "auth",
			InstanceId: "auth-1",
			Env:        "prod",
			Version:    "1.0.0",
		},
		Kernel: kernel.Config{
			Language: "go",
			Version:  "1.25.1",
		},
		Service: service.Config{
			Name:          "auth",
			Namespace:     "default",
			Type:          "svc",
			ClusterDomain: "cluster.local",
			Weight:        100,
		},
		Protocol:   "grpc",
		ServerPort: 9090,
	}
}

func benchmarkGatewayManifest(serviceCount, methodsPerService int) *GatewayManifest {
	manifest := &GatewayManifest{
		Schema:        GatewayManifestSchema,
		DescriptorRef: "https://minio.lhdht.cn/descriptor/bench/v0.0.1.pb",
		Services:      make([]GatewayManifestService, 0, serviceCount),
		Routes:        make([]HTTPRoute, 0, serviceCount),
	}
	for serviceIndex := 0; serviceIndex < serviceCount; serviceIndex++ {
		serviceName := fmt.Sprintf("acme.bench.v1.Service%d", serviceIndex)
		methods := make([]string, 0, methodsPerService)
		for methodIndex := 0; methodIndex < methodsPerService; methodIndex++ {
			methods = append(methods, fmt.Sprintf("/%s/Method%d", serviceName, methodIndex))
		}
		manifest.Services = append(manifest.Services, GatewayManifestService{
			Name:    serviceName,
			Methods: methods,
		})
		manifest.Routes = append(manifest.Routes, HTTPRoute{
			ID:         fmt.Sprintf("%s.Method0.http0", serviceName),
			HTTPMethod: "GET",
			Path:       fmt.Sprintf("/v1/bench/%d", serviceIndex),
			FullMethod: fmt.Sprintf("/%s/Method0", serviceName),
		})
	}
	if err := manifest.NormalizeAndValidate(); err != nil {
		panic(err)
	}
	return manifest
}

func BenchmarkGatewayManifestMethodPaths(b *testing.B) {
	b.ReportAllocs()

	cases := []struct {
		name             string
		serviceCount     int
		methodsPerServer int
	}{
		{name: "small", serviceCount: 1, methodsPerServer: 8},
		{name: "medium", serviceCount: 8, methodsPerServer: 16},
		{name: "large", serviceCount: 32, methodsPerServer: 32},
	}

	for _, tc := range cases {
		manifest := benchmarkGatewayManifest(tc.serviceCount, tc.methodsPerServer)
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = manifest.MethodPaths()
			}
		})
	}
}

func BenchmarkNewServiceNode(b *testing.B) {
	b.ReportAllocs()
	options := benchmarkServiceOptions()
	manifest := benchmarkGatewayManifest(8, 16)

	for i := 0; i < b.N; i++ {
		_ = NewServiceNode(options, manifest)
	}
}

func BenchmarkControllerOnConnected(b *testing.B) {
	b.ReportAllocs()
	controller, err := NewController(benchmarkNoopClient{}, NewServiceNode(benchmarkServiceOptions(), benchmarkGatewayManifest(4, 8)))
	if err != nil {
		b.Fatalf("new controller failed: %v", err)
	}
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		if err := controller.OnConnected(ctx); err != nil {
			b.Fatalf("on connected failed: %v", err)
		}
	}
}

func BenchmarkEmitWatchEvent(b *testing.B) {
	b.ReportAllocs()

	b.Run("structured", func(b *testing.B) {
		ctx := context.Background()
		payload := []string{`{"event":"connected","message":"ready","service":"sidecar-agent","status":"ready"}`}
		events := make(chan ConnectionEvent, 1)
		for i := 0; i < b.N; i++ {
			if err := emitWatchEvent(ctx, events, "connected", "1", payload); err != nil {
				b.Fatalf("emit watch event failed: %v", err)
			}
			<-events
		}
	})

	b.Run("legacy", func(b *testing.B) {
		ctx := context.Background()
		payload := []string{"ok"}
		events := make(chan ConnectionEvent, 1)
		for i := 0; i < b.N; i++ {
			if err := emitWatchEvent(ctx, events, "message", "1", payload); err != nil {
				b.Fatalf("emit watch event failed: %v", err)
			}
			<-events
		}
	})
}
