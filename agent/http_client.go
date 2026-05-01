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

// HttpClient 提供一个基于 JSON over HTTP 的本地 agent client 实现。
type HttpClient struct {
	// baseURL 表示本机 sidecar-agent 管理接口前缀。
	baseURL string
	// client 表示实际发起 HTTP 请求的客户端。
	client *http.Client
}

// NewHttpClient 创建一个新的本地 HTTP JSON client。
func NewHttpClient(baseURL string, timeout time.Duration) *HttpClient {
	// 对基础地址做最小清洗，避免拼接路径时出现双斜杠。
	cleanBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	// 返回一个带固定超时的最小 HTTP 客户端封装。
	return &HttpClient{
		// 保存规范化后的基础地址，供后续直接拼接路径。
		baseURL: cleanBaseURL,
		client: &http.Client{
			// 为每次管理接口调用设置统一超时上限。
			Timeout: timeout,
		},
	}
}

// PostJSON 把任意请求对象编码成 JSON 并发往本机 sidecar-agent。
func (c *HttpClient) PostJSON(ctx context.Context, path string, payload any) error {
	// 先把请求体对象编码成 JSON。
	body, err := json.Marshal(payload)
	// 如果编码失败，说明请求模型本身有问题，直接返回。
	if err != nil {
		return err
	}
	// 基于基础地址和路径构造最终请求 URL。
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	// 如果请求对象构造失败，则直接把错误返回给上层。
	if err != nil {
		return err
	}
	// 显式标记请求体为 JSON。
	request.Header.Set("Content-Type", "application/json")
	// 发送请求到本机 sidecar-agent。
	response, err := c.client.Do(request)
	// 如果底层网络请求失败，则直接返回底层错误。
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
