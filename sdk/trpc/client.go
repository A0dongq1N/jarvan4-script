// Package trpc 提供压测脚本使用的腾讯 tRPC over HTTP 客户端。
//
// tRPC over HTTP 协议说明：
//   - Method: POST
//   - URL: http://{host}/{package}.{Service}/{Method}
//     例：http://localhost:8080/trpc.user.UserService/GetUser
//   - Request Content-Type: application/json
//   - Response: {"code":0,"msg":"","data":{...}} 或 {"code":非0,"msg":"错误信息"}
//
// 自动调用 ctx.Recorder 上报每次调用的耗时和结果，
// label 格式为 "/{package}.{Service}/{Method}"（与 URL path 保持一致）。
//
// 典型用法：
//
//	// Setup 中创建客户端（所有 VU 共享）
//	cli := trpc.New(ctx, ctx.Vars.Env("TRPC_ADDR"))
//
//	// Default 中调用
//	var resp GetOrderResp
//	err := cli.Call(ctx, "trpc.order.OrderService", "GetOrder",
//	    map[string]interface{}{"order_id": "123"}, &resp)
package trpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/A0dongq1N/jarvan4-platform/spec"
)

// Client tRPC over HTTP 客户端。
type Client struct {
	baseURL    string
	httpClient *http.Client
	headers    map[string]string
}

// Option 客户端选项函数。
type Option func(*Client)

// WithTimeout 设置单次调用超时（默认 10s）。
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

// WithHeader 添加默认请求头（如鉴权 token）。
func WithHeader(key, value string) Option {
	return func(c *Client) { c.headers[key] = value }
}

// WithHeaders 批量添加默认请求头。
func WithHeaders(headers map[string]string) Option {
	return func(c *Client) {
		for k, v := range headers {
			c.headers[k] = v
		}
	}
}

// New 创建 tRPC over HTTP 客户端。
// baseURL 为服务地址，如 "http://localhost:8080"。
func New(ctx *spec.RunContext, baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		headers: make(map[string]string),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// trpcResponse tRPC over HTTP 统一响应体结构
type trpcResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
}

// Call 调用 tRPC 方法，自动上报指标。
//
//   serviceName: tRPC 服务全名，如 "trpc.user.UserService"
//   method:      方法名，如 "GetUser"
//   req:         请求体（会被序列化为 JSON）
//   resp:        响应 data 字段会被反序列化到此结构，传 nil 忽略响应体
//
// URL 拼接规则：{baseURL}/{serviceName}/{method}
// 示例：http://localhost:8080/trpc.user.UserService/GetUser
func (c *Client) Call(ctx *spec.RunContext, serviceName, method string, req interface{}, resp interface{}) error {
	path := "/" + serviceName + "/" + method
	url := c.baseURL + path

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("trpc marshal req: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("trpc new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}

	start := time.Now()
	httpResp, err := c.httpClient.Do(httpReq)
	duration := time.Since(start)

	if err != nil {
		record(ctx, path, duration, err)
		return fmt.Errorf("trpc call %s: %w", path, err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		record(ctx, path, duration, err)
		return fmt.Errorf("trpc read response: %w", err)
	}

	// HTTP 层错误
	if httpResp.StatusCode != http.StatusOK {
		callErr := &spec.ScriptError{
			Type:    "system",
			Code:    fmt.Sprintf("HTTP_%d", httpResp.StatusCode),
			Message: fmt.Sprintf("tRPC HTTP %d: %s", httpResp.StatusCode, string(respBody)),
			API:     path,
		}
		record(ctx, path, duration, callErr)
		return callErr
	}

	// 解析 tRPC 响应体
	var trpcResp trpcResponse
	if err := json.Unmarshal(respBody, &trpcResp); err != nil {
		record(ctx, path, duration, err)
		return fmt.Errorf("trpc parse response: %w", err)
	}

	// tRPC 业务错误（code != 0）
	if trpcResp.Code != 0 {
		callErr := &spec.ScriptError{
			Type:    "business",
			Code:    fmt.Sprintf("%d", trpcResp.Code),
			Message: trpcResp.Message,
			API:     path,
		}
		record(ctx, path, duration, callErr)
		return callErr
	}

	record(ctx, path, duration, nil)

	// 反序列化 data 字段
	if resp != nil && len(trpcResp.Data) > 0 {
		if err := json.Unmarshal(trpcResp.Data, resp); err != nil {
			return fmt.Errorf("trpc unmarshal data: %w", err)
		}
	}
	return nil
}

// CallRaw 调用 tRPC 方法，返回原始 data JSON（不反序列化）。
// 适用于响应结构动态变化，或只关心是否成功的场景。
func (c *Client) CallRaw(ctx *spec.RunContext, serviceName, method string, req interface{}) (json.RawMessage, error) {
	var raw json.RawMessage
	err := c.Call(ctx, serviceName, method, req, &raw)
	return raw, err
}

func record(ctx *spec.RunContext, label string, duration time.Duration, err error) {
	if ctx != nil && ctx.Recorder != nil {
		ctx.Recorder.Record(label, duration, err)
	}
}
