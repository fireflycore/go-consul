package agent

import (
	"context"
	"testing"

	"github.com/fireflycore/go-micro/app"
	"github.com/fireflycore/go-micro/kernel"
	"github.com/fireflycore/go-micro/service"
)

// fakeClient 记录控制器发往本机 agent 的调用轨迹。
type fakeClient struct {
	registerCalls   []*ServiceNode
	drainCalls      []DrainRequest
	deregisterCalls []DeregisterRequest
}

func (c *fakeClient) Register(ctx context.Context, request *ServiceNode) error {
	c.registerCalls = append(c.registerCalls, request)
	return nil
}

func (c *fakeClient) Drain(ctx context.Context, request DrainRequest) error {
	c.drainCalls = append(c.drainCalls, request)
	return nil
}

func (c *fakeClient) Deregister(ctx context.Context, request DeregisterRequest) error {
	c.deregisterCalls = append(c.deregisterCalls, request)
	return nil
}

func testServiceNode(name string, port uint) *ServiceNode {
	fullMethod := "/acme." + name + ".v1.Service/Ping"
	return &ServiceNode{
		ServiceOptions: &ServiceOptions{
			App: app.Config{
				Id:         "app-1",
				InstanceId: "app-1-1",
				Name:       name,
				Env:        "prod",
				Version:    "1.0.0",
			},
			Kernel: kernel.Config{
				Language: "go",
				Version:  "1.25.1",
			},
			Service: service.Config{
				Name:          name,
				Namespace:     "default",
				Type:          "svc",
				ClusterDomain: "cluster.local",
				Weight:        100,
			},
			Protocol:   "grpc",
			ServerPort: port,
		},
		DNS:           name + ".default.svc.cluster.local",
		Methods:       []string{fullMethod},
		DescriptorRef: "https://minio.lhdht.cn/descriptor/" + name + "/v0.0.1.pb",
		HTTPRoutes: []HTTPRoute{
			{
				ID:         "acme." + name + ".v1.Service.Ping.http0",
				HTTPMethod: "GET",
				Path:       "/v1/" + name + "/ping",
				FullMethod: fullMethod,
			},
		},
	}
}

func testGatewayManifest() *GatewayManifest {
	return &GatewayManifest{
		Schema:        GatewayManifestSchema,
		DescriptorRef: "https://minio.lhdht.cn/descriptor/auth/v0.0.1.pb",
		Services: []GatewayManifestService{
			{
				Name: "acme.auth.v1.AuthService",
				Methods: []string{
					"/acme.auth.v1.AuthService/Login",
					"/acme.auth.v1.AuthService/Logout",
				},
			},
		},
		Routes: []HTTPRoute{
			{
				ID:         "acme.auth.v1.AuthService.Login.http0",
				HTTPMethod: "GET",
				Path:       "/v1/auth/login",
				FullMethod: "/acme.auth.v1.AuthService/Login",
			},
		},
	}
}

// TestControllerOnConnectedReplaysRegister 验证控制器在连接建立时会重放注册。
func TestControllerOnConnectedReplaysRegister(t *testing.T) {
	client := &fakeClient{}
	controller, err := NewController(client, testServiceNode("auth", 9090))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	if err := controller.OnConnected(context.Background()); err != nil {
		t.Fatalf("on connected failed: %v", err)
	}
	if err := controller.OnConnected(context.Background()); err != nil {
		t.Fatalf("second on connected failed: %v", err)
	}
	if got, want := len(client.registerCalls), 2; got != want {
		t.Fatalf("unexpected register call count: got=%d want=%d", got, want)
	}
	status := controller.Status()
	if !status.Connected || !status.Registered {
		t.Fatal("expected controller to be connected and registered")
	}
}

// TestControllerDrainAndDeregisterUseServiceNode 验证控制器会直接基于 ServiceNode 构造摘流与注销请求。
func TestControllerDrainAndDeregisterUseServiceNode(t *testing.T) {
	client := &fakeClient{}
	controller, err := NewController(client, testServiceNode("payment", 8080))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	if err := controller.Drain(context.Background(), "20s"); err != nil {
		t.Fatalf("drain failed: %v", err)
	}
	if err := controller.Deregister(context.Background()); err != nil {
		t.Fatalf("deregister failed: %v", err)
	}
	if got, want := len(client.drainCalls), 1; got != want {
		t.Fatalf("unexpected drain call count: got=%d want=%d", got, want)
	}
	if got, want := len(client.deregisterCalls), 1; got != want {
		t.Fatalf("unexpected deregister call count: got=%d want=%d", got, want)
	}
	if got, want := client.drainCalls[0].AppId, "app-1"; got != want {
		t.Fatalf("unexpected drain app id: got=%s want=%s", got, want)
	}
	if got, want := client.deregisterCalls[0].AppInstanceId, "app-1-1"; got != want {
		t.Fatalf("unexpected deregister app instance id: got=%s want=%s", got, want)
	}
}
