package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

// sidecarRawResponseEnvelope 表示 sidecar 管理接口统一返回的 JSON envelope。
type sidecarRawResponseEnvelope struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SidecarResponseMeta 描述一次管理接口响应的公共元信息。
type SidecarResponseMeta struct {
	StatusCode int
	Success    bool
	Code       string
	Message    string
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
	_, err := c.doJSONRequest(ctx, http.MethodPost, path, payload)
	return err
}

// doJSONRequest 统一处理 sidecar-agent 的 JSON envelope 请求与错误解析。
func (c *HttpClient) doJSONRequest(ctx context.Context, method, path string, payload any) (SidecarResponseMeta, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return SidecarResponseMeta{}, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return SidecarResponseMeta{}, err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return SidecarResponseMeta{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return SidecarResponseMeta{}, err
	}
	meta, parseErr := decodeSidecarResponse(responseBody)
	meta.StatusCode = response.StatusCode
	if parseErr != nil {
		return meta, parseErr
	}
	if response.StatusCode == http.StatusOK {
		return meta, nil
	}
	return meta, &SidecarAPIError{
		Method:     method,
		Path:       path,
		StatusCode: response.StatusCode,
		Status:     response.Status,
		Code:       meta.Code,
		Message:    meta.Message,
	}
}

// decodeSidecarResponse 解析 sidecar 的响应 envelope，并提取公共元信息。
func decodeSidecarResponse(body []byte) (SidecarResponseMeta, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return SidecarResponseMeta{}, nil
	}
	var envelope sidecarRawResponseEnvelope
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return SidecarResponseMeta{}, nil
	}
	return SidecarResponseMeta{
		Success: envelope.Success,
		Code:    envelope.Code,
		Message: envelope.Message,
	}, nil
}
