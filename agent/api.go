package agent

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// ApiClient 提供一个基于 JSON over HTTP 的本地 agent client 实现。
type ApiClient struct {
	// client 表示实际发起 HTTP 请求的客户端。
	client *JSONHTTPClient
}

// NewApiClient 创建一个新的本地 HTTP JSON client。
func NewApiClient(baseURL string, timeout time.Duration) *JSONHTTPClient {
	// 对基础地址做最小清洗，避免拼接路径时出现双斜杠。
	cleanBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	// 返回一个带固定超时的最小 HTTP 客户端封装。
	return &JSONHTTPClient{
		baseURL: cleanBaseURL,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Register 通过本机管理接口注册当前服务。
func (c *ApiClient) Register(ctx context.Context, request ServiceNodeInfo) error {
	// 统一走公共 POST JSON 逻辑，减少重复实现。
	return c.client.postJSON(ctx, "/register", request)
}

// Drain 通过本机管理接口发起摘流。
func (c *ApiClient) Drain(ctx context.Context, request DrainRequest) error {
	// 统一走公共 POST JSON 逻辑，减少重复实现。
	return c.client.postJSON(ctx, "/drain", request)
}

// Deregister 通过本机管理接口发起注销。
func (c *ApiClient) Deregister(ctx context.Context, request DeregisterRequest) error {
	// 统一走公共 POST JSON 逻辑，减少重复实现。
	return c.client.postJSON(ctx, "/deregister", request)
}
