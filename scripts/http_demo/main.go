//go:build plugin

// 压测脚本：HTTP GET 接口压测示例
// 目标接口：httpbin.org/get（公开测试接口，无需鉴权）
package main

import (
	"fmt"

	"github.com/A0dongq1N/jarvan4-platform/spec"
	sdkhttp "github.com/A0dongq1N/jarvan4-script/sdk/http"
)

// Script 导出符号，Worker 通过 plugin.Lookup("Script") 获取
// 必须声明为接口类型，否则 plugin.Lookup 返回 **HttpDemoScript 导致类型断言失败
var Script spec.ScriptEntry = &HttpDemoScript{}

// PluginABI 插件 ABI 版本，须与 Worker 内 spec.PluginABIVersion 一致
var PluginABI = spec.PluginABIVersion

type HttpDemoScript struct{}

type setupData struct {
	HTTP    *sdkhttp.Client
	BaseURL string
}

func (s *HttpDemoScript) Setup(ctx *spec.RunContext) (interface{}, error) {
	baseURL := ctx.Vars.Env("BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("BASE_URL 环境变量未配置")
	}
	ctx.Log.Info("Setup 完成，目标地址：%s", baseURL)
	return &setupData{
		HTTP:    sdkhttp.New(ctx),
		BaseURL: baseURL,
	}, nil
}

func (s *HttpDemoScript) Default(ctx *spec.RunContext) error {
	sd := ctx.SetupData.(*setupData)

	res, err := sd.HTTP.Get(ctx, sd.BaseURL+"/get", spec.WithQuery("vu", fmt.Sprintf("%d", ctx.VUId)))
	if err != nil {
		return err
	}

	ctx.Check.That(res).Status(200).RTLt(2000)
	return nil
}

func (s *HttpDemoScript) Teardown(ctx *spec.RunContext, data interface{}) error {
	if sd, ok := data.(*setupData); ok && sd.HTTP != nil {
		sd.HTTP.Close()
	}
	ctx.Log.Info("Teardown 完成")
	return nil
}
