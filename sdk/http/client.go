// Package http 提供压测脚本使用的 HTTP 客户端。
// 所有请求自动调用 ctx.Recorder 上报耗时和成功/失败指标，脚本无需手动记录。
package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/A0dongq1N/jarvan4-platform/spec"
)

// Client HTTP 客户端，实现 spec.HTTPClient 接口。
// 通过 New(ctx) 创建，持有对 RunContext 的引用以自动上报指标。
type Client struct {
	ctx        *spec.RunContext
	httpClient *http.Client
}

// New 创建绑定到 ctx 的 HTTP 客户端。
// timeout 为单次请求超时，0 表示不限制。
func New(ctx *spec.RunContext, timeout time.Duration) *Client {
	return &Client{
		ctx: ctx,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Get(rawURL string, opts ...spec.RequestOption) (*spec.HTTPResponse, error) {
	req := &spec.HTTPRequest{Method: "GET", URL: rawURL}
	for _, o := range opts {
		o(req)
	}
	return c.Do(req)
}

func (c *Client) Post(rawURL string, body interface{}, opts ...spec.RequestOption) (*spec.HTTPResponse, error) {
	req := &spec.HTTPRequest{Method: "POST", URL: rawURL, Body: body}
	for _, o := range opts {
		o(req)
	}
	return c.Do(req)
}

func (c *Client) Put(rawURL string, body interface{}, opts ...spec.RequestOption) (*spec.HTTPResponse, error) {
	req := &spec.HTTPRequest{Method: "PUT", URL: rawURL, Body: body}
	for _, o := range opts {
		o(req)
	}
	return c.Do(req)
}

func (c *Client) Delete(rawURL string, opts ...spec.RequestOption) (*spec.HTTPResponse, error) {
	req := &spec.HTTPRequest{Method: "DELETE", URL: rawURL}
	for _, o := range opts {
		o(req)
	}
	return c.Do(req)
}

func (c *Client) Do(req *spec.HTTPRequest) (*spec.HTTPResponse, error) {
	start := time.Now()
	resp, err := c.do(req)
	duration := time.Since(start)

	label := req.Name
	if label == "" {
		label = normalizePath(req.URL)
	}

	if c.ctx != nil && c.ctx.Recorder != nil {
		var recordErr error
		if err != nil {
			recordErr = err
		} else if resp != nil && resp.StatusCode >= 400 {
			recordErr = fmt.Errorf("http %d", resp.StatusCode)
		}
		c.ctx.Recorder.Record(label, duration, recordErr)
		if resp != nil && resp.IsSkipped() {
			c.ctx.Recorder.Skip()
		}
	}

	return resp, err
}

func (c *Client) do(req *spec.HTTPRequest) (*spec.HTTPResponse, error) {
	// 序列化请求体
	var bodyReader io.Reader
	if req.Body != nil {
		switch v := req.Body.(type) {
		case []byte:
			bodyReader = bytes.NewReader(v)
		case string:
			bodyReader = strings.NewReader(v)
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("marshal body: %w", err)
			}
			bodyReader = bytes.NewReader(b)
		}
	}

	// 构建请求 URL（附加 query 参数）
	rawURL := req.URL
	if len(req.Query) > 0 {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, fmt.Errorf("parse url: %w", err)
		}
		q := u.Query()
		for k, v := range req.Query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		rawURL = u.String()
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), req.Method, rawURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	// 设置 Content-Type 默认值
	if req.Body != nil {
		if _, ok := req.Headers["Content-Type"]; !ok {
			httpReq.Header.Set("Content-Type", "application/json")
		}
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	// 设置单次请求超时
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel := context.WithTimeout(httpReq.Context(), req.Timeout)
		defer cancel()
		httpReq = httpReq.WithContext(ctx)
	}

	start := time.Now()
	httpResp, err := c.httpClient.Do(httpReq)
	duration := time.Since(start)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	headers := make(map[string][]string)
	for k, v := range httpResp.Header {
		headers[k] = v
	}

	return &spec.HTTPResponse{
		StatusCode: httpResp.StatusCode,
		Headers:    headers,
		Body:       body,
		Duration:   duration,
	}, nil
}

// RequestOption 构造函数

func WithHeader(key, value string) spec.RequestOption {
	return func(r *spec.HTTPRequest) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers[key] = value
	}
}

func WithHeaders(headers map[string]string) spec.RequestOption {
	return func(r *spec.HTTPRequest) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		for k, v := range headers {
			r.Headers[k] = v
		}
	}
}

func WithQuery(key, value string) spec.RequestOption {
	return func(r *spec.HTTPRequest) {
		if r.Query == nil {
			r.Query = make(map[string]string)
		}
		r.Query[key] = value
	}
}

func WithTimeout(d time.Duration) spec.RequestOption {
	return func(r *spec.HTTPRequest) {
		r.Timeout = d
	}
}

func WithBasicAuth(user, pass string) spec.RequestOption {
	return func(r *spec.HTTPRequest) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Authorization"] = basicAuthHeader(user, pass)
	}
}

// WithName 显式指定指标 label（URL pattern），用于报告接口维度统计归类。
// 对于路径中含非数字动态段（slug、hash 等）必须手动指定，否则每个值成为独立 pattern。
func WithName(pattern string) spec.RequestOption {
	return func(r *spec.HTTPRequest) {
		r.Name = pattern
	}
}

// normalizePath 将 URL 路径中的纯数字段和 UUID 替换为占位符，用于指标聚合。
// 示例：/api/goods/123 → /api/goods/:id
//
//	/api/user/550e8400-e29b-41d4-a716-446655440000 → /api/user/:uuid
var (
	reDigit = regexp.MustCompile(`/\d+`)
	reUUID  = regexp.MustCompile(`/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
)

func normalizePath(rawURL string) string {
	// 只取 path 部分
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	p := u.Path
	p = reUUID.ReplaceAllString(p, "/:uuid")
	p = reDigit.ReplaceAllString(p, "/:id")
	return p
}

func basicAuthHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}
