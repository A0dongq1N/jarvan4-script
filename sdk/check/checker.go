// Package check 提供 spec.Checker 接口的标准实现，由 Worker 注入到 RunContext。
package check

import "github.com/Aodongq1n/jarvan4-platform/spec"

// Checker 实现 spec.Checker 接口。
type Checker struct{}

// New 创建 Checker 实例（Worker 在初始化 RunContext 时调用）。
func New() *Checker { return &Checker{} }

// That 对 HTTP 响应创建断言链。
func (c *Checker) That(res *spec.HTTPResponse) *spec.Assertion {
	return spec.NewAssertion(res)
}
