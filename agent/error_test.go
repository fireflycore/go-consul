package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"testing"
)

// 错误可能在 SDK 或调用方合并，只有所有分支均为 sidecar 清理网络失败时才能降级。
func TestIsShutdownConnectionError(t *testing.T) {
	connectionErr := &url.Error{
		Op: "Post", URL: "http://127.0.0.1:18600/drain",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
	}
	cleanupErr := &AgentRunError{Stage: AgentRunStageAgentShutdown, Err: connectionErr}
	serveErr := &AgentRunError{Stage: AgentRunStageServe, Err: connectionErr}
	protocolErr := errors.New("invalid sidecar response")
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "no error"},
		{name: "nil run error", err: (*AgentRunError)(nil)},
		{name: "nil network error", err: &AgentRunError{Stage: AgentRunStageAgentShutdown, Err: (*net.OpError)(nil)}},
		{name: "connection refused", err: cleanupErr, want: true},
		{name: "wrapped cleanup", err: fmt.Errorf("run: %w", cleanupErr), want: true},
		{name: "request timeout", err: &AgentRunError{Stage: AgentRunStageAgentShutdown, Err: &url.Error{Op: "Post", Err: context.DeadlineExceeded}}, want: true},
		{name: "joined cleanup", err: errors.Join(cleanupErr, cleanupErr), want: true},
		{name: "joined network causes", err: &AgentRunError{Stage: AgentRunStageAgentShutdown, Err: errors.Join(connectionErr, connectionErr)}, want: true},
		{name: "serve failure", err: serveErr},
		{name: "lifecycle failure", err: &AgentRunError{Stage: AgentRunStageLifecycle, Err: connectionErr}},
		{name: "local shutdown failure", err: &AgentRunError{Stage: AgentRunStageShutdown, Err: connectionErr}},
		{name: "unstaged network failure", err: connectionErr},
		{name: "cancellation", err: context.Canceled},
		{name: "mixed serve failure", err: errors.Join(cleanupErr, serveErr)},
		{name: "mixed cancellation", err: errors.Join(context.Canceled, cleanupErr)},
		{name: "nested serve failure", err: &AgentRunError{Stage: AgentRunStageAgentShutdown, Err: errors.Join(connectionErr, serveErr)}},
		{name: "mixed nested cause", err: &AgentRunError{Stage: AgentRunStageAgentShutdown, Err: errors.Join(connectionErr, protocolErr)}},
		{name: "protocol failure", err: &AgentRunError{Stage: AgentRunStageAgentShutdown, Err: protocolErr}},
		{name: "API rejection", err: &AgentRunError{Stage: AgentRunStageAgentShutdown, Err: &SidecarAPIError{StatusCode: http.StatusBadRequest}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsShutdownConnectionError(tt.err); got != tt.want {
				t.Fatalf("classification = %v, want %v; error: %v", got, tt.want, tt.err)
			}
		})
	}
}
