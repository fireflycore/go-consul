package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

type retryClient struct {
	fakeClient
	failure  error
	attempts chan *ServiceNode
}

func (c *retryClient) Register(_ context.Context, node *ServiceNode) error {
	c.registerCalls = append(c.registerCalls, node)
	c.attempts <- node
	if len(c.registerCalls) == 1 {
		return c.failure
	}
	return nil
}

// 登记暂时失败后，不依赖新的 connected 或 heartbeat 事件也能取得成功回执。
func TestRunnerRetriesRegistrationWithoutReconnect(t *testing.T) {
	client := &retryClient{failure: &SidecarAPIError{StatusCode: http.StatusServiceUnavailable}, attempts: make(chan *ServiceNode, 4)}
	controller, err := NewController(client, testServiceNode("auth", 9090))
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan ConnectionEvent, 1)
	events <- ConnectionEvent{Type: ConnectionEventTypeConnected, Connected: true}
	runner, _ := NewRunner(&fakeEventSource{events: events}, controller, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	var nodes []*ServiceNode
	for len(nodes) < 2 {
		select {
		case node := <-client.attempts:
			nodes = append(nodes, node)
		case <-time.After(3 * time.Second):
			t.Fatal("registration was not retried")
		}
	}
	// 等待成功状态落地，避免在 client 回包之前读取状态。
	deadline := time.Now().Add(time.Second)
	for !controller.Status().Registered && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	status := controller.Status()
	if nodes[0] != nodes[1] || !status.Registered || !status.Connected || status.DisconnectCount != 0 || status.RegisterReplayFailureCount != 1 || status.LastError != "" {
		t.Fatalf("unexpected recovery: nodes=%v status=%+v", nodes, status)
	}
}

// 服务端已接管、客户端未等到响应时，用相同实例身份重试并补回成功回执。
func TestRunnerRecoversAfterCommittedRequestTimesOut(t *testing.T) {
	var mu sync.Mutex
	instances := make(map[string]bool)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var node ServiceNode
		if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		instances[node.App.InstanceId] = true
		requests++
		first := requests == 1
		mu.Unlock()
		if first {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"code":"registered"}`))
	}))
	defer server.Close()
	controller, _ := NewController(NewApiClient(NewHttpClient(server.URL, 30*time.Millisecond)), testServiceNode("auth", 9090))
	events := make(chan ConnectionEvent, 1)
	events <- ConnectionEvent{Type: ConnectionEventTypeConnected, Connected: true}
	runner, _ := NewRunner(&fakeEventSource{events: events}, controller, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	deadline := time.Now().Add(3 * time.Second)
	for !controller.Status().Registered && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	mu.Lock()
	count, unique := requests, len(instances)
	mu.Unlock()
	status := controller.Status()
	if count != 2 || unique != 1 || !status.Registered || status.LastError != "" || status.RegisterReplayFailureCount != 1 {
		t.Fatalf("unexpected timeout recovery: requests=%d instances=%d status=%+v", count, unique, status)
	}
}

// 明确拒绝不会继续定时重试，下一次真正重连仍可重新确认。
func TestRunnerDoesNotRetryRejectedRegistration(t *testing.T) {
	client := &retryClient{failure: &SidecarAPIError{StatusCode: http.StatusConflict}, attempts: make(chan *ServiceNode, 4)}
	controller, _ := NewController(client, testServiceNode("auth", 9090))
	events := make(chan ConnectionEvent, 1)
	events <- ConnectionEvent{Type: ConnectionEventTypeConnected, Connected: true}
	runner, _ := NewRunner(&fakeEventSource{events: events}, controller, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	<-client.attempts
	select {
	case <-client.attempts:
		t.Fatal("rejected registration was retried")
	case <-time.After(1200 * time.Millisecond):
	}
	cancel()
	<-done
}

func TestHeartbeatDoesNotChangeConnectionTimes(t *testing.T) {
	controller, _ := NewController(&fakeClient{}, testServiceNode("auth", 9090))
	if err := controller.OnConnected(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := controller.Status()
	controller.ObserveEvent(ConnectionEvent{Type: ConnectionEventTypeHeartbeat})
	after := controller.Status()
	if after.LastDisconnectedAt != "" || after.LastConnectedAt != before.LastConnectedAt || !after.Connected {
		t.Fatalf("heartbeat changed connection state: %+v", after)
	}
}

type cancelRegisterClient struct {
	entered       chan struct{}
	returned      chan struct{}
	order         []string
	drainErr      error
	deregisterErr error
}

func (c *cancelRegisterClient) Register(ctx context.Context, _ *ServiceNode) error {
	close(c.entered)
	<-ctx.Done()
	c.order = append(c.order, "register canceled")
	close(c.returned)
	return ctx.Err()
}

func (c *cancelRegisterClient) Drain(context.Context, DrainRequest) error {
	c.order = append(c.order, "drain")
	return c.drainErr
}

func (c *cancelRegisterClient) Deregister(context.Context, DeregisterRequest) error {
	c.order = append(c.order, "deregister")
	return c.deregisterErr
}

// 业务自行退出时根上下文仍有效，Agent 仍须先取消并等注册结束，再执行下线。
func TestRunWaitsForRegistrationBeforeCleanup(t *testing.T) {
	client := &cancelRegisterClient{entered: make(chan struct{}), returned: make(chan struct{})}
	controller, _ := NewController(client, testServiceNode("auth", 9090))
	events := make(chan ConnectionEvent, 1)
	events <- ConnectionEvent{Type: ConnectionEventTypeConnected, Connected: true}
	runner, _ := NewRunner(&fakeEventSource{events: events}, controller, nil)
	a := &Agent{
		Controller: controller, Runner: runner, gracePeriod: "1s",
		serve:    func(context.Context) error { <-client.entered; return nil },
		shutdown: func(context.Context) error { client.order = append(client.order, "business shutdown"); return nil },
	}
	done := make(chan error, 1)
	go func() { done <- a.Run(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not cancel registration before cleanup")
	}
	want := []string{"register canceled", "drain", "business shutdown", "deregister"}
	if !reflect.DeepEqual(client.order, want) {
		t.Fatalf("unexpected cleanup order: %v", client.order)
	}
	if status := a.Status(); status.Connected || status.Registered {
		t.Fatalf("unexpected stopped state: %+v", status)
	}
}

// 每一步错误都保留阶段信息；摘流或业务关闭失败不阻止最终注销。
func TestFinishRunContinuesAfterCleanupErrors(t *testing.T) {
	drainErr, shutdownErr, deregisterErr, serveErr := errors.New("drain"), errors.New("shutdown"), errors.New("deregister"), errors.New("serve")
	client := &cancelRegisterClient{drainErr: drainErr, deregisterErr: deregisterErr}
	controller, _ := NewController(client, testServiceNode("auth", 9090))
	a := &Agent{Controller: controller, gracePeriod: "1s", shutdown: func(context.Context) error {
		client.order = append(client.order, "business shutdown")
		return shutdownErr
	}}
	err := a.finishRun(serveErr)
	for _, want := range []error{drainErr, shutdownErr, deregisterErr, serveErr} {
		if !errors.Is(err, want) {
			t.Fatalf("missing failure %v in %v", want, err)
		}
	}
	if !reflect.DeepEqual(client.order, []string{"drain", "business shutdown", "deregister"}) || IsShutdownConnectionError(err) {
		t.Fatalf("unexpected cleanup result: order=%v err=%v", client.order, err)
	}
}
