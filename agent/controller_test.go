package agent

import (
	"context"
	"testing"
)

// fakeProvider 提供固定注册描述，便于验证控制器行为。
type fakeProvider struct {
	// request 保存测试中预期返回的注册请求。
	request RegisterRequest
}

// BuildRegisterRequest 返回测试固定注册描述。
func (p fakeProvider) BuildRegisterRequest(ctx context.Context) (RegisterRequest, error) {
	return p.request, nil
}

// fakeClient 记录控制器发往本机 agent 的调用轨迹。
type fakeClient struct {
	// registerCalls 记录所有 register 调用。
	registerCalls []RegisterRequest
	// drainCalls 记录所有 drain 调用。
	drainCalls []DrainRequest
	// deregisterCalls 记录所有 deregister 调用。
	deregisterCalls []DeregisterRequest
}

// Register 记录 register 调用。
func (c *fakeClient) Register(ctx context.Context, request RegisterRequest) error {
	c.registerCalls = append(c.registerCalls, request)
	return nil
}

// Drain 记录 drain 调用。
func (c *fakeClient) Drain(ctx context.Context, request DrainRequest) error {
	c.drainCalls = append(c.drainCalls, request)
	return nil
}

// Deregister 记录 deregister 调用。
func (c *fakeClient) Deregister(ctx context.Context, request DeregisterRequest) error {
	c.deregisterCalls = append(c.deregisterCalls, request)
	return nil
}

// TestControllerOnConnectedReplaysRegister 验证控制器在连接建立时会重放注册。
func TestControllerOnConnectedReplaysRegister(t *testing.T) {
	// 创建最小 fake client，用来记录 register 调用次数。
	client := &fakeClient{}
	// 创建一个带固定注册描述的控制器。
	controller, err := NewController(client, fakeProvider{
		request: RegisterRequest{
			Name: "auth",
			Port: 9090,
		},
	})
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	// 连续两次模拟连接恢复，第二次应再次重放 register。
	if err := controller.OnConnected(context.Background()); err != nil {
		t.Fatalf("on connected failed: %v", err)
	}
	if err := controller.OnConnected(context.Background()); err != nil {
		t.Fatalf("second on connected failed: %v", err)
	}
	if got, want := len(client.registerCalls), 2; got != want {
		t.Fatalf("unexpected register call count: got=%d want=%d", got, want)
	}
	// 注册成功后，控制器状态应落在 connected + registered。
	status := controller.Status()
	if !status.Connected || !status.Registered {
		t.Fatal("expected controller to be connected and registered")
	}
}

// TestControllerDrainAndDeregisterUseLastRequest 验证控制器会复用最后一次成功注册的描述。
func TestControllerDrainAndDeregisterUseLastRequest(t *testing.T) {
	// 创建最小 fake client，用来记录 drain 与 deregister 调用。
	client := &fakeClient{}
	// 创建一个带固定注册描述的控制器。
	controller, err := NewController(client, fakeProvider{
		request: RegisterRequest{
			Name: "payment",
			Port: 8080,
		},
	})
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	// 先完成一次成功连接，建立 last register request。
	if err := controller.OnConnected(context.Background()); err != nil {
		t.Fatalf("on connected failed: %v", err)
	}
	// 再分别执行 drain 与 deregister。
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
	if got, want := client.drainCalls[0].Name, "payment"; got != want {
		t.Fatalf("unexpected drain service: got=%s want=%s", got, want)
	}
	if got, want := client.deregisterCalls[0].Port, 8080; got != want {
		t.Fatalf("unexpected deregister port: got=%d want=%d", got, want)
	}
}
