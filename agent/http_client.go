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
	Success     bool            `json:"success"`
	Code        string          `json:"code"`
	Message     string          `json:"message"`
	Data        json.RawMessage `json:"data"`
	GeneratedAt time.Time       `json:"generated_at"`
}

// SidecarResponseMeta 描述一次管理接口响应的公共元信息。
type SidecarResponseMeta struct {
	StatusCode  int
	Success     bool
	Code        string
	Message     string
	GeneratedAt time.Time
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
	_, err := c.doJSONRequest(ctx, http.MethodPost, path, payload, nil, http.StatusOK)
	return err
}

// GetJSON 向本机 sidecar-agent 发起 GET 请求，并把 data 字段解码到 target。
func (c *HttpClient) GetJSON(ctx context.Context, path string, target any, acceptedStatusCodes ...int) (SidecarResponseMeta, error) {
	return c.doJSONRequest(ctx, http.MethodGet, path, nil, target, acceptedStatusCodes...)
}

// doJSONRequest 统一处理 sidecar-agent 的 JSON envelope 请求与错误解析。
func (c *HttpClient) doJSONRequest(ctx context.Context, method, path string, payload any, target any, acceptedStatusCodes ...int) (SidecarResponseMeta, error) {
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
	meta, parseErr := decodeSidecarResponse(responseBody, target)
	meta.StatusCode = response.StatusCode
	if parseErr != nil {
		return meta, parseErr
	}
	if isAcceptedStatus(response.StatusCode, acceptedStatusCodes...) {
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

// decodeSidecarResponse 解析 sidecar 的响应 envelope，并把 data 解码进 target。
func decodeSidecarResponse(body []byte, target any) (SidecarResponseMeta, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return SidecarResponseMeta{}, nil
	}
	var envelope sidecarRawResponseEnvelope
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		if target == nil {
			return SidecarResponseMeta{}, nil
		}
		return SidecarResponseMeta{}, err
	}
	meta := SidecarResponseMeta{
		Success:     envelope.Success,
		Code:        envelope.Code,
		Message:     envelope.Message,
		GeneratedAt: envelope.GeneratedAt,
	}
	if target != nil && len(bytes.TrimSpace(envelope.Data)) > 0 {
		if err := json.Unmarshal(envelope.Data, target); err != nil {
			return meta, err
		}
	}
	return meta, nil
}

// isAcceptedStatus 判断当前响应状态码是否属于调用方允许的结果。
func isAcceptedStatus(statusCode int, acceptedStatusCodes ...int) bool {
	if len(acceptedStatusCodes) == 0 {
		return statusCode == http.StatusOK
	}
	for _, accepted := range acceptedStatusCodes {
		if statusCode == accepted {
			return true
		}
	}
	return false
}
