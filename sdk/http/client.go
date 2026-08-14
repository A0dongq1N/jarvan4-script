// Package http 提供压测脚本使用的 HTTP 客户端。
// 所有请求自动调用 ctx.Recorder 上报耗时和成功/失败指标，脚本无需手动记录。
//
// 典型用法：
//
//	// Setup 中创建客户端（所有 VU 共享连接池）
//	client := sdkhttp.New(ctx)
//
//	// Default 中发起请求（必须传入当前迭代的 RunContext）
//	res, err := client.Get(ctx, baseURL+"/api/health", spec.WithName("/api/health"))
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/A0dongq1N/jarvan4-platform/spec"
)

// Client HTTP 客户端。应在 Setup 中创建一次，经 SetupData 分发给所有 VU，以复用连接池。
type Client struct {
	httpClient *http.Client
}

// ClientOption 客户端选项。
type ClientOption func(*clientConfig)

type clientConfig struct {
	timeout time.Duration
}

// WithTimeout 设置默认单次请求超时（可被 spec.WithTimeout 覆盖）。
func WithTimeout(d time.Duration) ClientOption {
	return func(c *clientConfig) { c.timeout = d }
}

// New 创建共享连接池的 HTTP 客户端。
// 若未指定 WithTimeout，则读取 ctx.Vars.Env("TIMEOUT_MS")（由 Worker 注入场景超时）；
// 仍为空时默认 30s。
func New(ctx *spec.RunContext, opts ...ClientOption) *Client {
	cfg := &clientConfig{timeout: 30 * time.Second}
	if ctx != nil && ctx.Vars != nil {
		if ms := ctx.Vars.Env("TIMEOUT_MS"); ms != "" {
			if v, err := strconv.Atoi(ms); err == nil && v > 0 {
				cfg.timeout = time.Duration(v) * time.Millisecond
			}
		}
	}
	for _, o := range opts {
		o(cfg)
	}
	return &Client{
		httpClient: &http.Client{
			Timeout: cfg.timeout,
			Transport: &http.Transport{
				MaxIdleConns:          4096,
				MaxIdleConnsPerHost:   2048,
				MaxConnsPerHost:       0, // 不限制，由发压池控制并发
				IdleConnTimeout:       90 * time.Second,
				DisableCompression:    true, // 压测路径省 CPU
				ForceAttemptHTTP2:     false,
				ResponseHeaderTimeout: cfg.timeout,
			},
		},
	}
}

// Close 关闭空闲连接，可在 Teardown 中调用。
func (c *Client) Close() {
	if c == nil || c.httpClient == nil {
		return
	}
	if t, ok := c.httpClient.Transport.(*http.Transport); ok {
		t.CloseIdleConnections()
	}
}

// Get 发起 GET 请求。
func (c *Client) Get(ctx *spec.RunContext, rawURL string, opts ...spec.RequestOption) (*spec.HTTPResponse, error) {
	req := &spec.HTTPRequest{Method: "GET", URL: rawURL}
	for _, o := range opts {
		o(req)
	}
	return c.Do(ctx, req)
}

// Post 发起 POST 请求。
func (c *Client) Post(ctx *spec.RunContext, rawURL string, body interface{}, opts ...spec.RequestOption) (*spec.HTTPResponse, error) {
	req := &spec.HTTPRequest{Method: "POST", URL: rawURL, Body: body}
	for _, o := range opts {
		o(req)
	}
	return c.Do(ctx, req)
}

// Put 发起 PUT 请求。
func (c *Client) Put(ctx *spec.RunContext, rawURL string, body interface{}, opts ...spec.RequestOption) (*spec.HTTPResponse, error) {
	req := &spec.HTTPRequest{Method: "PUT", URL: rawURL, Body: body}
	for _, o := range opts {
		o(req)
	}
	return c.Do(ctx, req)
}

// Delete 发起 DELETE 请求。
func (c *Client) Delete(ctx *spec.RunContext, rawURL string, opts ...spec.RequestOption) (*spec.HTTPResponse, error) {
	req := &spec.HTTPRequest{Method: "DELETE", URL: rawURL}
	for _, o := range opts {
		o(req)
	}
	return c.Do(ctx, req)
}

// Do 发起自定义请求，并上报指标、写入 ctx.LastAPILabel。
func (c *Client) Do(ctx *spec.RunContext, req *spec.HTTPRequest) (*spec.HTTPResponse, error) {
	start := time.Now()
	resp, err := c.do(req)
	duration := time.Since(start)

	label := req.Name
	if label == "" {
		label = normalizePath(req.URL)
	}
	if ctx != nil {
		ctx.LastAPILabel = label
	}

	if ctx != nil && ctx.Recorder != nil {
		var recordErr error
		if err != nil {
			recordErr = err
		} else if resp != nil && resp.StatusCode >= 400 {
			recordErr = spec.BuildHTTPError(resp.StatusCode, resp.Body, label)
		}
		ctx.Recorder.Record(label, ctx.ScriptName, duration, recordErr)
		if resp != nil && resp.IsSkipped() {
			ctx.Recorder.Skip()
		}
	}

	return resp, err
}

func (c *Client) do(req *spec.HTTPRequest) (*spec.HTTPResponse, error) {
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

	if req.Body != nil {
		if _, ok := req.Headers["Content-Type"]; !ok {
			httpReq.Header.Set("Content-Type", "application/json")
		}
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	if req.Timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel := context.WithTimeout(httpReq.Context(), req.Timeout)
		defer cancel()
		httpReq = httpReq.WithContext(reqCtx)
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

// normalizePath 将 URL 路径中的纯数字段和 UUID 替换为占位符，用于指标聚合。
var (
	reDigit = regexp.MustCompile(`/\d+`)
	reUUID  = regexp.MustCompile(`/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
)

func normalizePath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	p := u.Path
	p = reUUID.ReplaceAllString(p, "/:uuid")
	p = reDigit.ReplaceAllString(p, "/:id")
	return p
}
