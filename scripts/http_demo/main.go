// 压测脚本：HTTP GET 接口压测示例
// 目标接口：httpbin.org/get（公开测试接口，无需鉴权）
package main

import (
	"fmt"

	sdkhttp "github.com/Aodongq1n/jarvan4-platform/sdk/http"
	"github.com/Aodongq1n/jarvan4-platform/sdk/spec"
)

// Script 导出符号，Worker 通过 plugin.Lookup("Script") 获取
// 必须声明为接口类型，否则 plugin.Lookup 返回 **HttpDemoScript 导致类型断言失败
var Script spec.ScriptEntry = &HttpDemoScript{}

type HttpDemoScript struct{}

func (s *HttpDemoScript) Setup(ctx *spec.RunContext) (interface{}, error) {
	baseURL := ctx.Vars.Env("BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("BASE_URL 环境变量未配置")
	}
	ctx.Log.Info("Setup 完成，目标地址：%s", baseURL)
	return nil, nil
}

func (s *HttpDemoScript) Default(ctx *spec.RunContext) error {
	baseURL := ctx.Vars.Env("BASE_URL")

	res, err := ctx.HTTP.Get(baseURL+"/get", sdkhttp.WithQuery("vu", fmt.Sprintf("%d", ctx.VUId)))
	if err != nil {
		return err
	}

	ctx.Check.That(res).Status(200).RTLt(2000)
	return nil
}

func (s *HttpDemoScript) Teardown(ctx *spec.RunContext, data interface{}) error {
	ctx.Log.Info("Teardown 完成")
	return nil
}
