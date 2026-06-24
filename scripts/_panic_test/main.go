//go:build plugin

// Package main 是一个故意 panic 的测试脚本,用于验证 Worker 引擎的 recover 机制。
// W-20 测试用例:验证脚本 panic 不会导致 worker 进程崩溃。
package main

import (
	"github.com/Aodongq1n/jarvan4-platform/sdk/spec"
)

// Script 导出符号(Worker 通过 plugin.Lookup("Script") 获取)
var Script spec.ScriptEntry = &PanicScript{}

// PanicScript 故意在 Default 中 panic
type PanicScript struct{}

func (s *PanicScript) Setup(ctx *spec.RunContext) (interface{}, error) {
	ctx.Log.Info("PanicScript Setup: about to run panic test")
	return nil, nil
}

func (s *PanicScript) Default(ctx *spec.RunContext) error {
	// 故意 panic,验证引擎 recover 不崩
	panic("e2e panic test: this should be recovered by the engine")
}

func (s *PanicScript) Teardown(ctx *spec.RunContext, data interface{}) error {
	ctx.Log.Info("PanicScript Teardown: survived the panic")
	return nil
}
