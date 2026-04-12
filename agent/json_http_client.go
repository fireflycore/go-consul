package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// JSONHTTPClient 提供一个基于 JSON over HTTP 的本地 agent client 实现。
type JSONHTTPClient struct {
	// baseURL 表示本机 sidecar-agent 管理接口前缀。
	baseURL string
	// client 表示实际发起 HTTP 请求的客户端。
	client *http.Client
}

// NewJSONHTTPClient 创建一个新的本地 HTTP JSON client。
func NewJSONHTTPClient(baseURL string, timeout time.Duration) *JSONHTTPClient {
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
func (c *JSONHTTPClient) Register(ctx context.Context, request RegisterRequest) error {
	// 统一走公共 POST JSON 逻辑，减少重复实现。
	return c.postJSON(ctx, "/register", request)
}

// Drain 通过本机管理接口发起摘流。
func (c *JSONHTTPClient) Drain(ctx context.Context, request DrainRequest) error {
	// 统一走公共 POST JSON 逻辑，减少重复实现。
	return c.postJSON(ctx, "/drain", request)
}

// Deregister 通过本机管理接口发起注销。
func (c *JSONHTTPClient) Deregister(ctx context.Context, request DeregisterRequest) error {
	// 统一走公共 POST JSON 逻辑，减少重复实现。
	return c.postJSON(ctx, "/deregister", request)
}

// postJSON 把任意请求对象编码成 JSON 并发往本机 sidecar-agent。
func (c *JSONHTTPClient) postJSON(ctx context.Context, path string, payload any) error {
	// 先把请求体对象编码成 JSON。
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// 基于基础地址和路径构造最终请求 URL。
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	// 显式标记请求体为 JSON。
	request.Header.Set("Content-Type", "application/json")
	// 发送请求到本机 sidecar-agent。
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	// 在函数结束前关闭响应体，避免连接泄漏。
	defer response.Body.Close()
	// 仅接受 200 成功状态码，其他情况统一返回错误。
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("agent request failed: %s %s returned %s", http.MethodPost, path, response.Status)
	}
	// 成功时返回 nil。
	return nil
}
