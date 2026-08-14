//go:build plugin

// 压测脚本：可控错误率接口，用于验证熔断保护（全局 / 接口级规则）
// 环境变量：
//   BASE_URL   被压测服务地址，需支持 GET /api/error（如 jarvan4-script/scripts/_target）
//   ERROR_RATE 错误率 0~1，默认 1.0（100% 失败，便于快速触发熔断）
package main

import (
	"fmt"
	"strconv"

	"github.com/A0dongq1N/jarvan4-platform/spec"
)

var Script spec.ScriptEntry = &HttpErrorScript{}

type HttpErrorScript struct{}

func (s *HttpErrorScript) Setup(ctx *spec.RunContext) (interface{}, error) {
	baseURL := ctx.Vars.Env("BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("BASE_URL 环境变量未配置")
	}
	rate := parseErrorRate(ctx.Vars.Env("ERROR_RATE"))
	ctx.Log.Info("Setup 完成，目标 %s，错误率 %.0f%%", baseURL, rate*100)
	return nil, nil
}

func (s *HttpErrorScript) Default(ctx *spec.RunContext) error {
	baseURL := ctx.Vars.Env("BASE_URL")
	rate := parseErrorRate(ctx.Vars.Env("ERROR_RATE"))

	url := fmt.Sprintf("%s/api/error?rate=%s", baseURL, formatRate(rate))
	res, err := ctx.HTTP.Get(url, spec.WithName("/api/error"))
	if err != nil {
		return err
	}
	ctx.Check.That(res).Status(200).RTLt(3000)
	return nil
}

func (s *HttpErrorScript) Teardown(ctx *spec.RunContext, data interface{}) error {
	ctx.Log.Info("Teardown 完成")
	return nil
}

func parseErrorRate(raw string) float64 {
	if raw == "" {
		return 1.0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		return 1.0
	}
	if v > 1 {
		return 1.0
	}
	return v
}

func formatRate(rate float64) string {
	return strconv.FormatFloat(rate, 'f', 2, 64)
}
