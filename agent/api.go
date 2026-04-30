package agent

import (
	"context"
)

// ApiClient 提供一个基于 JSON over HTTP 的本地 agent client 实现。
type ApiClient struct {
	// client 表示实际发起 HTTP 请求的客户端。
	client *HttpClient
}

// NewApiClient 创建一个新的本地 HTTP JSON client。
func NewApiClient(client *HttpClient) *ApiClient {
	return &ApiClient{
		client: client,
	}
}

// Register 通过本机管理接口注册当前服务。
func (c *ApiClient) Register(ctx context.Context, request *ServiceNode) error {
	// 统一走公共 POST JSON 逻辑，减少重复实现。
	return c.client.PostJSON(ctx, "/register", request)
}

// Drain 通过本机管理接口发起摘流。
func (c *ApiClient) Drain(ctx context.Context, request DrainRequest) error {
	// 统一走公共 POST JSON 逻辑，减少重复实现。
	return c.client.PostJSON(ctx, "/drain", request)
}

// Deregister 通过本机管理接口发起注销。
func (c *ApiClient) Deregister(ctx context.Context, request DeregisterRequest) error {
	// 统一走公共 POST JSON 逻辑，减少重复实现。
	return c.client.PostJSON(ctx, "/deregister", request)
}
