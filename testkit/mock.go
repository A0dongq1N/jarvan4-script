package testkit

import (
	"fmt"
	"time"

	"github.com/Aodongq1n/jarvan4-platform/sdk/spec"
)

// MockResponse 预设的 HTTP 响应。
type MockResponse struct {
	StatusCode int
	Body       []byte
	Headers    map[string][]string
	Err        error // 非 nil 时模拟网络错误
}

// MockHTTPClient 模拟 HTTP 客户端，按 URL 前缀匹配返回预设响应。
type MockHTTPClient struct {
	// Responses key 为 URL（完整 URL 或路径前缀），value 为预设响应
	Responses map[string]*MockResponse
	// Requests 记录所有发出的请求，供测试断言
	Requests []*spec.HTTPRequest
}

// NewMockHTTP 创建 MockHTTPClient。
func NewMockHTTP() *MockHTTPClient {
	return &MockHTTPClient{
		Responses: make(map[string]*MockResponse),
	}
}

// On 注册 URL 对应的预设响应。
func (m *MockHTTPClient) On(url string, resp *MockResponse) *MockHTTPClient {
	if m.Responses == nil {
		m.Responses = make(map[string]*MockResponse)
	}
	m.Responses[url] = resp
	return m
}

func (m *MockHTTPClient) Get(rawURL string, opts ...spec.RequestOption) (*spec.HTTPResponse, error) {
	req := &spec.HTTPRequest{Method: "GET", URL: rawURL}
	for _, o := range opts {
		o(req)
	}
	return m.Do(req)
}

func (m *MockHTTPClient) Post(rawURL string, body interface{}, opts ...spec.RequestOption) (*spec.HTTPResponse, error) {
	req := &spec.HTTPRequest{Method: "POST", URL: rawURL, Body: body}
	for _, o := range opts {
		o(req)
	}
	return m.Do(req)
}

func (m *MockHTTPClient) Put(rawURL string, body interface{}, opts ...spec.RequestOption) (*spec.HTTPResponse, error) {
	req := &spec.HTTPRequest{Method: "PUT", URL: rawURL, Body: body}
	for _, o := range opts {
		o(req)
	}
	return m.Do(req)
}

func (m *MockHTTPClient) Delete(rawURL string, opts ...spec.RequestOption) (*spec.HTTPResponse, error) {
	req := &spec.HTTPRequest{Method: "DELETE", URL: rawURL}
	for _, o := range opts {
		o(req)
	}
	return m.Do(req)
}

func (m *MockHTTPClient) Do(req *spec.HTTPRequest) (*spec.HTTPResponse, error) {
	m.Requests = append(m.Requests, req)

	// 精确匹配
	if mock, ok := m.Responses[req.URL]; ok {
		if mock.Err != nil {
			return nil, mock.Err
		}
		return &spec.HTTPResponse{
			StatusCode: mock.StatusCode,
			Body:       mock.Body,
			Headers:    mock.Headers,
			Duration:   time.Millisecond * 10,
		}, nil
	}

	// 默认返回 200 空 body
	return &spec.HTTPResponse{
		StatusCode: 200,
		Body:       []byte(`{}`),
		Duration:   time.Millisecond * 10,
	}, nil
}

// RequestCount 返回发出的请求总数。
func (m *MockHTTPClient) RequestCount() int { return len(m.Requests) }

// LastRequest 返回最后一次请求。
func (m *MockHTTPClient) LastRequest() *spec.HTTPRequest {
	if len(m.Requests) == 0 {
		return nil
	}
	return m.Requests[len(m.Requests)-1]
}

// RequestAt 返回第 i 次请求（0-based）。
func (m *MockHTTPClient) RequestAt(i int) *spec.HTTPRequest {
	if i < 0 || i >= len(m.Requests) {
		return nil
	}
	return m.Requests[i]
}

// OK 快速创建 200 响应。
func OK(body []byte) *MockResponse {
	return &MockResponse{StatusCode: 200, Body: body}
}

// Status 快速创建指定状态码响应。
func Status(code int) *MockResponse {
	return &MockResponse{StatusCode: code, Body: []byte(`{}`)}
}

// NetworkError 快速创建网络错误响应。
func NetworkError(err error) *MockResponse {
	return &MockResponse{Err: err}
}

// MockLogger 记录日志供测试断言。
type MockLogger struct {
	Logs []string
}

func (l *MockLogger) Debug(format string, args ...interface{}) { l.log("DEBUG", format, args...) }
func (l *MockLogger) Info(format string, args ...interface{})  { l.log("INFO", format, args...) }
func (l *MockLogger) Warn(format string, args ...interface{})  { l.log("WARN", format, args...) }
func (l *MockLogger) Error(format string, args ...interface{}) { l.log("ERROR", format, args...) }
func (l *MockLogger) log(level, format string, args ...interface{}) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.Logs = append(l.Logs, "["+level+"] "+msg)
}

// MockSleeper 记录睡眠调用，不真正等待。
type MockSleeper struct {
	Calls []time.Duration
}

func (s *MockSleeper) Sleep(d time.Duration) {
	s.Calls = append(s.Calls, d)
}

// MockRecorder 记录指标上报，供测试验证。
type MockRecorder struct {
	Records []RecordEntry
}

type RecordEntry struct {
	Label    string
	Duration time.Duration
	Err      error
}

func (r *MockRecorder) Record(label string, duration time.Duration, err error) {
	r.Records = append(r.Records, RecordEntry{Label: label, Duration: duration, Err: err})
}

func (r *MockRecorder) Skip() {}

// SuccessCount 返回成功记录数。
func (r *MockRecorder) SuccessCount() int {
	count := 0
	for _, rec := range r.Records {
		if rec.Err == nil {
			count++
		}
	}
	return count
}

// FailCount 返回失败记录数。
func (r *MockRecorder) FailCount() int {
	count := 0
	for _, rec := range r.Records {
		if rec.Err != nil {
			count++
		}
	}
	return count
}
