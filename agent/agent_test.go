package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	microapp "github.com/fireflycore/go-micro/app"
	"github.com/fireflycore/go-micro/kernel"
	"github.com/fireflycore/go-micro/service"
	"google.golang.org/grpc"
)

func TestDefaultSidecarAgentConfig(t *testing.T) {
	config := DefaultSidecarAgentConfig("")
	if got, want := config.BaseURL, DefaultAdminBaseURL; got != want {
		t.Fatalf("unexpected base url: got=%s want=%s", got, want)
	}
	if got, want := config.WatchURL, DefaultAdminBaseURL+DefaultWatchPath; got != want {
		t.Fatalf("unexpected watch url: got=%s want=%s", got, want)
	}
}

func TestNewBuildsServiceNodeAndNormalizesDefaults(t *testing.T) {
	agent, err := New(&ServiceOptions{
		App: microapp.Config{
			Id:         "10001",
			Name:       "auth",
			Secret:     "sensitive",
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
	}, SidecarAgentConfig{
		RawServices: []*grpc.ServiceDesc{
			{
				ServiceName: "acme.auth.v1.AuthService",
				Methods: []grpc.MethodDesc{
					{MethodName: "Login"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("new agent failed: %v", err)
	}
	if got, want := agent.Client.client.baseURL, DefaultAdminBaseURL; got != want {
		t.Fatalf("unexpected base url: got=%s want=%s", got, want)
	}
	if got, want := agent.Source.watchURL, DefaultAdminBaseURL+DefaultWatchPath; got != want {
		t.Fatalf("unexpected watch url: got=%s want=%s", got, want)
	}
	if got, want := len(agent.Node.Methods), 1; got != want {
		t.Fatalf("unexpected method count: got=%d want=%d", got, want)
	}
	if got, want := agent.Node.Methods[0], "/acme.auth.v1.AuthService/Login"; got != want {
		t.Fatalf("unexpected method path: got=%s want=%s", got, want)
	}
	if agent.Node.App.Secret != "" {
		t.Fatal("expected app secret to be scrubbed from service node")
	}
}

func TestNewRejectsNilServiceOptions(t *testing.T) {
	agent, err := New(nil, SidecarAgentConfig{})
	if err == nil {
		t.Fatal("expected nil service options error")
	}
	if agent != nil {
		t.Fatalf("expected nil agent, got: %+v", agent)
	}
}

func TestNewReturnsControllerValidationError(t *testing.T) {
	agent, err := New(&ServiceOptions{
		App: microapp.Config{
			Id:         "10001",
			InstanceId: "broken-1",
		},
		Kernel: kernel.Config{
			Language: "go",
			Version:  "1.25.1",
		},
		ServerPort: 9090,
	}, SidecarAgentConfig{})
	if err == nil {
		t.Fatal("expected controller validation error")
	}
	if agent != nil {
		t.Fatalf("expected nil agent, got: %+v", agent)
	}
}

func TestAgentShutdownDelegatesToController(t *testing.T) {
	client := &fakeClient{}
	controller, err := NewController(client, testServiceNode("order", 7070))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	agent := &Agent{
		Controller:  controller,
		gracePeriod: "12s",
	}
	if err := agent.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	if got, want := len(client.drainCalls), 1; got != want {
		t.Fatalf("unexpected drain call count: got=%d want=%d", got, want)
	}
	if got, want := len(client.deregisterCalls), 1; got != want {
		t.Fatalf("unexpected deregister call count: got=%d want=%d", got, want)
	}
}

func TestAgentShutdownWithoutGracePeriodSkipsDrain(t *testing.T) {
	client := &fakeClient{}
	controller, err := NewController(client, testServiceNode("stock", 7071))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	agent := &Agent{Controller: controller}
	if err := agent.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	if got := len(client.drainCalls); got != 0 {
		t.Fatalf("expected no drain call, got=%d", got)
	}
	if got := len(client.deregisterCalls); got != 1 {
		t.Fatalf("expected one deregister call, got=%d", got)
	}
}

func TestAgentShutdownReturnsDrainError(t *testing.T) {
	drainErr := errors.New("drain failed")
	controller := &Controller{
		client: drainErrorClient{err: drainErr},
		node:   testServiceNode("stock", 7071),
	}
	agent := &Agent{
		Controller:  controller,
		gracePeriod: "5s",
	}
	if err := agent.Shutdown(context.Background()); !errors.Is(err, drainErr) {
		t.Fatalf("expected drain error, got: %v", err)
	}
}

func TestAgentStartWrapsRunnerError(t *testing.T) {
	controller, err := NewController(&fakeClient{}, testServiceNode("inventory", 5050))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	agent := &Agent{
		Controller: controller,
		Runner: &Runner{
			source:     failingEventSource{},
			controller: controller,
		},
		errors: make(chan error, 1),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	errCh := agent.Start(ctx)
	err = <-errCh
	var lifecycleErr *LifecycleRunError
	if !errors.As(err, &lifecycleErr) {
		t.Fatalf("expected LifecycleRunError, got: %v", err)
	}
	if !errors.Is(lifecycleErr, context.DeadlineExceeded) {
		t.Fatalf("expected wrapped deadline exceeded, got: %v", lifecycleErr)
	}
}

func TestAgentRunTriggersBusinessAndAgentShutdown(t *testing.T) {
	client := &fakeClient{}
	controller, err := NewController(client, testServiceNode("billing", 6060))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	shutdownCalled := false
	agent := &Agent{
		Controller: controller,
		Runner: &Runner{
			source: &fakeEventSource{
				events: make(chan ConnectionEvent),
			},
			controller: controller,
		},
		gracePeriod: "15s",
		serve: func(ctx context.Context) error {
			return nil
		},
		shutdown: func(ctx context.Context) error {
			shutdownCalled = true
			return nil
		},
		errors: make(chan error, 1),
	}
	if err := agent.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !shutdownCalled {
		t.Fatal("expected shutdown hook to be called")
	}
	if got, want := len(client.drainCalls), 1; got != want {
		t.Fatalf("unexpected drain call count: got=%d want=%d", got, want)
	}
	if got, want := len(client.deregisterCalls), 1; got != want {
		t.Fatalf("unexpected deregister call count: got=%d want=%d", got, want)
	}
}

func TestAgentRunWaitsForServeAfterLifecycleChannelClosed(t *testing.T) {
	client := &fakeClient{}
	controller, err := NewController(client, testServiceNode("settlement", 6062))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	releaseServe := make(chan struct{})
	agent := &Agent{
		Controller: controller,
		Runner: &Runner{
			source:     &fakeEventSource{events: closedConnectionEvents()},
			controller: controller,
		},
		gracePeriod: "7s",
		serve: func(ctx context.Context) error {
			<-releaseServe
			return nil
		},
		errors: make(chan error, 1),
	}
	done := make(chan error, 1)
	go func() {
		done <- agent.Run(context.Background())
	}()
	time.Sleep(30 * time.Millisecond)
	close(releaseServe)
	if err := <-done; err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if got := len(client.drainCalls); got != 1 {
		t.Fatalf("expected one drain call, got=%d", got)
	}
}

func TestAgentRunWrapsServeError(t *testing.T) {
	controller, err := NewController(&fakeClient{}, testServiceNode("catalog", 5050))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	serveErr := errors.New("serve failed")
	agent := &Agent{
		Controller: controller,
		Runner: &Runner{
			source: &fakeEventSource{
				events: make(chan ConnectionEvent),
			},
			controller: controller,
		},
		serve:  func(ctx context.Context) error { return serveErr },
		errors: make(chan error, 1),
	}
	err = agent.Run(context.Background())
	var runErr *AgentRunError
	if !errors.As(err, &runErr) {
		t.Fatalf("expected AgentRunError, got: %v", err)
	}
	if runErr.Stage != AgentRunStageServe || !errors.Is(runErr, serveErr) {
		t.Fatalf("unexpected run error: %+v", runErr)
	}
}

func TestAgentConfigureRunOverridesRuntimeHooks(t *testing.T) {
	controller, err := NewController(&fakeClient{}, testServiceNode("catalog", 5050))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	runner := &Runner{
		source:     failingEventSource{},
		controller: controller,
	}
	called := false
	agent := &Agent{
		Controller: controller,
		Runner:     runner,
		errors:     make(chan error, 1),
	}
	agent.ConfigureRun(SidecarAgentConfig{
		GracePeriod: "30s",
		OnError: func(ctx context.Context, err error) {
			called = true
		},
	})
	if got, want := agent.gracePeriod, "30s"; got != want {
		t.Fatalf("unexpected grace period: got=%s want=%s", got, want)
	}
	if runner.onError == nil {
		t.Fatal("expected runner onError to be configured")
	}
	runner.onError(context.Background(), errors.New("boom"))
	if !called {
		t.Fatal("expected configured onError to be invoked")
	}
}

func TestAgentConfigureRunOverridesServeAndShutdown(t *testing.T) {
	controller, err := NewController(&fakeClient{}, testServiceNode("profile", 6061))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	agent := &Agent{
		Controller: controller,
		Runner:     &Runner{controller: controller},
	}

	serveCalled := false
	shutdownCalled := false
	agent.ConfigureRun(SidecarAgentConfig{
		Serve: func(ctx context.Context) error {
			serveCalled = true
			return nil
		},
		Shutdown: func(ctx context.Context) error {
			shutdownCalled = true
			return nil
		},
		GracePeriod: "18s",
	})

	if got, want := agent.gracePeriod, "18s"; got != want {
		t.Fatalf("unexpected grace period: got=%s want=%s", got, want)
	}
	if err := agent.serve(context.Background()); err != nil {
		t.Fatalf("serve failed: %v", err)
	}
	if err := agent.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	if !serveCalled || !shutdownCalled {
		t.Fatalf("expected serve and shutdown to be replaced, got serve=%v shutdown=%v", serveCalled, shutdownCalled)
	}
}

func TestAgentDrainAndDeregisterDelegateToController(t *testing.T) {
	client := &fakeClient{}
	controller, err := NewController(client, testServiceNode("coupon", 8088))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	agent := &Agent{
		Controller:  controller,
		gracePeriod: "9s",
	}
	if err := agent.Drain(context.Background()); err != nil {
		t.Fatalf("drain failed: %v", err)
	}
	if err := agent.Deregister(context.Background()); err != nil {
		t.Fatalf("deregister failed: %v", err)
	}
	if got, want := len(client.drainCalls), 1; got != want {
		t.Fatalf("unexpected drain count: got=%d want=%d", got, want)
	}
	if got, want := client.drainCalls[0].GracePeriod, "9s"; got != want {
		t.Fatalf("unexpected grace period: got=%s want=%s", got, want)
	}
	if got, want := len(client.deregisterCalls), 1; got != want {
		t.Fatalf("unexpected deregister count: got=%d want=%d", got, want)
	}
}

func TestAgentRunReturnsLifecycleStageError(t *testing.T) {
	controller, err := NewController(&fakeClient{}, testServiceNode("ledger", 8181))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	agent := &Agent{
		Controller: controller,
		Runner: &Runner{
			source:     failingEventSource{},
			controller: controller,
		},
		errors: make(chan error, 1),
	}

	err = agent.Run(context.Background())
	var runErr *AgentRunError
	if !errors.As(err, &runErr) {
		t.Fatalf("expected AgentRunError, got: %v", err)
	}
	if runErr.Stage != AgentRunStageLifecycle {
		t.Fatalf("unexpected run error stage: %+v", runErr)
	}
}

func TestAgentStartRequiresRunner(t *testing.T) {
	agent := &Agent{}
	err := <-agent.Start(context.Background())
	var lifecycleErr *LifecycleRunError
	if !errors.As(err, &lifecycleErr) {
		t.Fatalf("expected LifecycleRunError, got: %v", err)
	}
	if lifecycleErr.Error() == "" {
		t.Fatal("expected non-empty lifecycle error text")
	}
}

// failingEventSource 用于模拟生命周期事件源初始化失败。
type failingEventSource struct{}

func (failingEventSource) Subscribe(ctx context.Context) (<-chan ConnectionEvent, error) {
	return nil, context.DeadlineExceeded
}

type drainErrorClient struct {
	err error
}

func (c drainErrorClient) Register(ctx context.Context, request *ServiceNode) error {
	return nil
}

func (c drainErrorClient) Drain(ctx context.Context, request DrainRequest) error {
	return c.err
}

func (c drainErrorClient) Deregister(ctx context.Context, request DeregisterRequest) error {
	return nil
}

func closedConnectionEvents() chan ConnectionEvent {
	ch := make(chan ConnectionEvent)
	close(ch)
	return ch
}
