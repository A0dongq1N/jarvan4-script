// Package testkit 提供脚本单元测试工具，无需真实网络、无需 Worker 环境。
package testkit

import (
	"context"

	"github.com/Aodongq1n/jarvan4-platform/sdk/check"
	"github.com/Aodongq1n/jarvan4-platform/sdk/spec"
	"github.com/Aodongq1n/jarvan4-platform/sdk/vars"
)

// Option RunContext 构造选项。
type Option func(*builder)

type builder struct {
	http      spec.HTTPClient
	envVars   map[string]string
	vuID      int
	iteration int64
	setupData interface{}
}

// NewContext 构造测试用 RunContext，所有外部依赖使用 mock 实现。
func NewContext(opts ...Option) *spec.RunContext {
	b := &builder{
		envVars: make(map[string]string),
	}
	for _, o := range opts {
		o(b)
	}

	mockHTTP := &MockHTTPClient{}
	if b.http != nil {
		mockHTTP = b.http.(*MockHTTPClient)
	}

	varStore := vars.New(b.envVars)
	checker := check.New()
	logger := &MockLogger{}
	sleeper := &MockSleeper{}
	recorder := &MockRecorder{}

	return &spec.RunContext{
		Context:   context.Background(),
		VUId:      b.vuID,
		Iteration: b.iteration,
		SetupData: b.setupData,
		HTTP:      mockHTTP,
		Check:     checker,
		Vars:      varStore,
		Log:       logger,
		Sleep:     sleeper,
		Recorder:  recorder,
	}
}

// WithHTTP 注入 MockHTTPClient（已预设响应）。
func WithHTTP(c *MockHTTPClient) Option {
	return func(b *builder) { b.http = c }
}

// WithEnv 设置环境变量（ctx.Vars.Env() 读取）。
func WithEnv(key, value string) Option {
	return func(b *builder) { b.envVars[key] = value }
}

// WithEnvMap 批量设置环境变量。
func WithEnvMap(env map[string]string) Option {
	return func(b *builder) {
		for k, v := range env {
			b.envVars[k] = v
		}
	}
}

// WithVUId 设置虚拟用户 ID。
func WithVUId(id int) Option {
	return func(b *builder) { b.vuID = id }
}

// WithIteration 设置迭代次数。
func WithIteration(i int64) Option {
	return func(b *builder) { b.iteration = i }
}

// WithSetupData 注入 Setup 返回的共享数据（测试 Default 时使用）。
func WithSetupData(data interface{}) Option {
	return func(b *builder) { b.setupData = data }
}
