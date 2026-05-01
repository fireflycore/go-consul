package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
)

func TestApiClientRoutesAllOperations(t *testing.T) {
	paths := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewApiClient(NewHttpClient(server.URL, time.Second))
	node := testServiceNode("auth", 9090)

	if err := client.Register(context.Background(), node); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if err := client.Drain(context.Background(), node.BuildDrainRequest("20s")); err != nil {
		t.Fatalf("drain failed: %v", err)
	}
	if err := client.Deregister(context.Background(), node.BuildDeregisterRequest()); err != nil {
		t.Fatalf("deregister failed: %v", err)
	}

	if got, want := len(paths), 3; got != want {
		t.Fatalf("unexpected path count: got=%d want=%d", got, want)
	}
	if paths[0] != "/register" || paths[1] != "/drain" || paths[2] != "/deregister" {
		t.Fatalf("unexpected paths: %+v", paths)
	}
}

func TestAgentGuardsAndStatus(t *testing.T) {
	var nilAgent *Agent
	if err := nilAgent.Drain(context.Background()); err == nil {
		t.Fatal("expected drain guard error")
	}
	if err := nilAgent.Deregister(context.Background()); err == nil {
		t.Fatal("expected deregister guard error")
	}
	if err := nilAgent.Shutdown(context.Background()); err == nil {
		t.Fatal("expected shutdown guard error")
	}
	if status := nilAgent.Status(); status != (Status{}) {
		t.Fatalf("unexpected status for nil agent: %+v", status)
	}
}

func TestAgentStatusDelegatesToController(t *testing.T) {
	controller, err := NewController(&fakeClient{}, testServiceNode("catalog", 5050))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	if err := controller.OnConnected(context.Background()); err != nil {
		t.Fatalf("on connected failed: %v", err)
	}
	agent := &Agent{Controller: controller}
	status := agent.Status()
	if !status.Connected || !status.Registered {
		t.Fatalf("unexpected delegated status: %+v", status)
	}
}

func TestErrorFormattingAndUnwrap(t *testing.T) {
	baseErr := errors.New("boom")

	replayErr := &RegisterReplayError{ServiceName: "auth", ServicePort: 9090, Err: baseErr}
	if replayErr.Error() == "" {
		t.Fatal("expected register replay error text")
	}
	if !errors.Is(replayErr, baseErr) {
		t.Fatal("expected replay error unwrap to match base error")
	}

	lifecycleErr := &LifecycleRunError{Err: baseErr}
	if lifecycleErr.Error() == "" {
		t.Fatal("expected lifecycle error text")
	}
	if !errors.Is(lifecycleErr, baseErr) {
		t.Fatal("expected lifecycle unwrap to match base error")
	}

	runErr := &AgentRunError{Stage: AgentRunStageServe, Err: baseErr}
	if runErr.Error() == "" {
		t.Fatal("expected run error text")
	}
	if !errors.Is(runErr, baseErr) {
		t.Fatal("expected run unwrap to match base error")
	}
}

func TestWatchErrorsAndRunnerValidation(t *testing.T) {
	httpStatusErr := &WatchHTTPStatusError{StatusCode: 500, Status: "500 Internal Server Error"}
	if httpStatusErr.Error() == "" {
		t.Fatal("expected watch http status error text")
	}
	if (&WatchHTTPStatusError{StatusCode: 503}).Error() == "" {
		t.Fatal("expected watch http status fallback text")
	}
	if (*WatchHTTPStatusError)(nil).Error() != "" {
		t.Fatal("expected nil watch http status error text to be empty")
	}

	parseErr := &WatchEventParseError{
		EventType: "connected",
		EventId:   "evt-1",
		Payload:   "{bad json}",
		Err:       errors.New("invalid character"),
	}
	if parseErr.Error() == "" {
		t.Fatal("expected watch parse error text")
	}
	if parseErr.Unwrap() == nil {
		t.Fatal("expected parse error unwrap")
	}
	if (&WatchEventParseError{EventType: "connected", Err: errors.New("bad")}).Error() == "" {
		t.Fatal("expected parse error fallback text")
	}
	if (*WatchEventParseError)(nil).Error() != "" {
		t.Fatal("expected nil parse error text to be empty")
	}
	if (*WatchEventParseError)(nil).Unwrap() != nil {
		t.Fatal("expected nil parse error unwrap to be nil")
	}

	if _, err := NewRunner(nil, &Controller{}, nil); err == nil {
		t.Fatal("expected nil source validation error")
	}
	if _, err := NewRunner(benchmarkEventSource{events: make(chan ConnectionEvent)}, nil, nil); err == nil {
		t.Fatal("expected nil controller validation error")
	}
}

func TestClassifyStatusErrorBranches(t *testing.T) {
	if got := classifyStatusError(nil); got != "" {
		t.Fatalf("unexpected nil error classification: %s", got)
	}
	if got := classifyStatusError(ErrWatchStreamClosed); got != "watch_stream_closed" {
		t.Fatalf("unexpected watch stream classification: %s", got)
	}
	if got := classifyStatusError(&WatchHTTPStatusError{StatusCode: 503}); got != "watch_http_status" {
		t.Fatalf("unexpected http status classification: %s", got)
	}
	if got := classifyStatusError(&WatchEventParseError{Err: errors.New("bad")}); got != "watch_event_parse" {
		t.Fatalf("unexpected parse classification: %s", got)
	}
	if got := classifyStatusError(&RegisterReplayError{Err: errors.New("bad")}); got != "register_replay" {
		t.Fatalf("unexpected replay classification: %s", got)
	}
	if got := classifyStatusError(errors.New("other")); got != "runtime_error" {
		t.Fatalf("unexpected runtime classification: %s", got)
	}
}

func TestServiceNodeRequestBuildersWithNil(t *testing.T) {
	var node *ServiceNode
	if request := node.BuildDrainRequest("10s"); request.GracePeriod != "10s" {
		t.Fatalf("unexpected nil drain request: %+v", request)
	}
	if request := node.BuildDeregisterRequest(); request != (DeregisterRequest{}) {
		t.Fatalf("unexpected nil deregister request: %+v", request)
	}
}

func TestNewControllerValidationAndHelpers(t *testing.T) {
	validNode := testServiceNode("auth", 9090)
	if _, err := NewController(nil, validNode); err == nil {
		t.Fatal("expected nil client validation error")
	}
	if _, err := NewController(&fakeClient{}, nil); err == nil {
		t.Fatal("expected nil node validation error")
	}
	if _, err := NewController(&fakeClient{}, &ServiceNode{}); err == nil {
		t.Fatal("expected nil service options validation error")
	}
	if _, err := NewController(&fakeClient{}, &ServiceNode{ServiceOptions: &ServiceOptions{ServerPort: 9090}}); err == nil {
		t.Fatal("expected missing service name validation error")
	}
	if _, err := NewController(&fakeClient{}, &ServiceNode{ServiceOptions: &ServiceOptions{Service: testServiceNode("auth", 9090).Service}}); err == nil {
		t.Fatal("expected missing port validation error")
	}

	controller, err := NewController(&fakeClient{}, validNode)
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	if got := formatStatusTime(time.Time{}); got != "" {
		t.Fatalf("unexpected zero format status time: %s", got)
	}
	if got, want := controller.Status().LastServiceName, "auth"; got != want {
		t.Fatalf("unexpected last service name: got=%s want=%s", got, want)
	}
}

func TestDefaultAndNormalizeSidecarConfigBranches(t *testing.T) {
	config := DefaultSidecarAgentConfig(" http://127.0.0.1:17000/ ")
	if got, want := config.BaseURL, "http://127.0.0.1:17000"; got != want {
		t.Fatalf("unexpected base url: got=%s want=%s", got, want)
	}

	normalized := normalizeSidecarAgentConfig(SidecarAgentConfig{
		BaseURL:           " http://127.0.0.1:18000/ ",
		WatchURL:          " http://127.0.0.1:19000/watch ",
		RequestTimeout:    time.Second,
		ReconnectInterval: 2 * time.Second,
	})
	if got, want := normalized.BaseURL, "http://127.0.0.1:18000"; got != want {
		t.Fatalf("unexpected normalized base url: got=%s want=%s", got, want)
	}
	if got, want := normalized.WatchURL, "http://127.0.0.1:19000/watch"; got != want {
		t.Fatalf("unexpected normalized watch url: got=%s want=%s", got, want)
	}
}

func TestHttpClientPostJSONErrorPaths(t *testing.T) {
	client := NewHttpClient("http://127.0.0.1:1", time.Millisecond)
	if err := client.PostJSON(context.Background(), "/register", map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("expected marshal error")
	}
	if err := NewHttpClient("://bad-url", time.Second).PostJSON(context.Background(), "/register", map[string]string{"ok": "1"}); err == nil {
		t.Fatal("expected request build error")
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	if err := NewHttpClient(server.URL, time.Second).PostJSON(context.Background(), "/register", map[string]string{"ok": "1"}); err == nil {
		t.Fatal("expected non-200 error")
	}
}

func TestAgentRunAndFinishBranches(t *testing.T) {
	controller, err := NewController(&fakeClient{}, testServiceNode("auth", 9090))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}

	runAgent := &Agent{
		Controller: controller,
		Runner: &Runner{
			source:     benchmarkEventSource{events: closedEventChannel()},
			controller: controller,
		},
		errors: make(chan error, 1),
	}
	if err := runAgent.Run(context.Background()); err != nil {
		t.Fatalf("expected run without serve to return nil, got: %v", err)
	}

	shutdownErr := errors.New("shutdown failed")
	agentWithShutdownErr := &Agent{
		Controller: controller,
		shutdown:   func(ctx context.Context) error { return shutdownErr },
	}
	if err := agentWithShutdownErr.finishRun(nil); !errors.Is(err, shutdownErr) {
		t.Fatalf("expected shutdown error, got: %v", err)
	}

	agentWithDeregisterErr := &Agent{
		Controller: &Controller{client: deregisterErrorClient{err: errors.New("deregister failed")}, node: testServiceNode("auth", 9090)},
	}
	if err := agentWithDeregisterErr.finishRun(nil); err == nil {
		t.Fatal("expected agent shutdown error")
	}
}

func TestErrorNilBranchesAndVariants(t *testing.T) {
	if (*RegisterReplayError)(nil).Error() != "" {
		t.Fatal("expected nil replay error text to be empty")
	}
	if (*RegisterReplayError)(nil).Unwrap() != nil {
		t.Fatal("expected nil replay unwrap to be nil")
	}
	if (&RegisterReplayError{Err: errors.New("boom")}).Error() == "" {
		t.Fatal("expected replay fallback error text")
	}
	if (&RegisterReplayError{ServiceName: "auth", Err: errors.New("boom")}).Error() == "" {
		t.Fatal("expected replay service-only error text")
	}

	if (*LifecycleRunError)(nil).Error() != "" {
		t.Fatal("expected nil lifecycle error text to be empty")
	}
	if (*LifecycleRunError)(nil).Unwrap() != nil {
		t.Fatal("expected nil lifecycle unwrap to be nil")
	}
	if (*AgentRunError)(nil).Error() != "" {
		t.Fatal("expected nil run error text to be empty")
	}
	if (*AgentRunError)(nil).Unwrap() != nil {
		t.Fatal("expected nil run unwrap to be nil")
	}
	if (&AgentRunError{Err: errors.New("boom")}).Error() == "" {
		t.Fatal("expected run fallback error text")
	}
}

func TestJoinDataLinesAndLooksLikeJSON(t *testing.T) {
	if got := joinDataLines(nil); got != "" {
		t.Fatalf("unexpected join result for nil: %q", got)
	}
	if got := joinDataLines([]string{"one"}); got != "one" {
		t.Fatalf("unexpected join result for single line: %q", got)
	}
	if got := joinDataLines([]string{"one", "two"}); got != "one\ntwo" {
		t.Fatalf("unexpected join result for two lines: %q", got)
	}
	if !looksLikeJSON("  {\"ok\":true}") {
		t.Fatal("expected json object detection")
	}
	if !looksLikeJSON(" [1,2] ") {
		t.Fatal("expected json array detection")
	}
	if looksLikeJSON("plain text") {
		t.Fatal("did not expect plain text to look like json")
	}
}

func TestBuildGRPCMethodsSkipsNilAndSorts(t *testing.T) {
	methods := BuildGRPCMethods([]*grpc.ServiceDesc{
		nil,
		{
			ServiceName: "acme.auth.v1.Auth",
			Methods: []grpc.MethodDesc{
				{MethodName: "Login"},
				{MethodName: "Logout"},
			},
		},
		{
			ServiceName: "acme.auth.v1.Admin",
			Methods: []grpc.MethodDesc{
				{MethodName: "Ban"},
			},
		},
	})
	if got, want := len(methods), 3; got != want {
		t.Fatalf("unexpected methods len: got=%d want=%d", got, want)
	}
	expected := []string{
		"/acme.auth.v1.Admin/Ban",
		"/acme.auth.v1.Auth/Login",
		"/acme.auth.v1.Auth/Logout",
	}
	for index, method := range expected {
		if methods[index] != method {
			t.Fatalf("unexpected method order at %d: got=%s want=%s", index, methods[index], method)
		}
	}
}

func TestControllerObserveEventAndRecordErrorBranches(t *testing.T) {
	controller, err := NewController(&fakeClient{}, testServiceNode("risk", 7171))
	if err != nil {
		t.Fatalf("new controller failed: %v", err)
	}
	generatedAt := time.Now().UTC()
	controller.ObserveEvent(ConnectionEvent{
		Type:        ConnectionEventTypeDisconnected,
		Connected:   false,
		EventId:     "evt-7",
		GeneratedAt: &generatedAt,
	})
	status := controller.Status()
	if got, want := status.LastEventId, "evt-7"; got != want {
		t.Fatalf("unexpected event id: got=%s want=%s", got, want)
	}
	if got := status.LastDisconnectedAt; got == "" {
		t.Fatal("expected disconnected time to be recorded")
	}
	controller.RecordError(nil)
	controller.RecordError(&WatchHTTPStatusError{StatusCode: 503})
	status = controller.Status()
	if got, want := status.LastErrorKind, "watch_http_status"; got != want {
		t.Fatalf("unexpected last error kind: got=%s want=%s", got, want)
	}
}

type deregisterErrorClient struct {
	err error
}

func (c deregisterErrorClient) Register(ctx context.Context, request *ServiceNode) error {
	return nil
}

func (c deregisterErrorClient) Drain(ctx context.Context, request DrainRequest) error {
	return nil
}

func (c deregisterErrorClient) Deregister(ctx context.Context, request DeregisterRequest) error {
	return c.err
}

func closedEventChannel() <-chan ConnectionEvent {
	ch := make(chan ConnectionEvent)
	close(ch)
	return ch
}

func TestWatchErrorStringsAreStable(t *testing.T) {
	if got := (&WatchHTTPStatusError{StatusCode: 418}).Error(); got == "" {
		t.Fatal("expected stable http status error text")
	}
	if got := (&WatchEventParseError{EventType: "heartbeat", Err: fmt.Errorf("bad")}).Error(); got == "" {
		t.Fatal("expected stable parse error text")
	}
}
