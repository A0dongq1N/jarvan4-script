//go:build plugin

// 压测脚本：登录 + 查询接口链路
// 环境变量：
//   BASE_URL   被压测服务地址，如 http://staging.example.com
//   USERNAME   登录账号（可选，默认 test_user_{VUId}）
//   PASSWORD   登录密码（可选，默认 password123）
package main

import (
	"fmt"

	sdkhttp "github.com/A0dongq1N/jarvan4-script/sdk/http"
	"github.com/A0dongq1N/jarvan4-platform/spec"
)

// Script 导出符号，Worker 通过 plugin.Lookup("Script") 获取。
var Script spec.ScriptEntry = &HttpLoginScript{}

type HttpLoginScript struct{}

type setupData struct {
	BaseURL string
}

func (s *HttpLoginScript) Setup(ctx *spec.RunContext) (interface{}, error) {
	baseURL := ctx.Vars.Env("BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("BASE_URL 未配置")
	}
	ctx.Log.Info("Setup: 目标地址 %s", baseURL)
	return &setupData{BaseURL: baseURL}, nil
}

func (s *HttpLoginScript) Default(ctx *spec.RunContext) error {
	sd := ctx.SetupData.(*setupData)

	// 首次迭代登录，token 复用
	if ctx.Vars.GetString("token") == "" {
		username := ctx.Vars.Env("USERNAME")
		if username == "" {
			username = fmt.Sprintf("test_user_%d", ctx.VUId)
		}
		password := ctx.Vars.Env("PASSWORD")
		if password == "" {
			password = "password123"
		}

		resp, err := ctx.HTTP.Post(
			sd.BaseURL+"/api/auth/login",
			map[string]string{"username": username, "password": password},
			sdkhttp.WithName("/api/auth/login"),
		)
		if err != nil {
			return err
		}

		if assertion := ctx.Check.That(resp).Status(200); assertion.Failed() {
			return &spec.ScriptError{
				Type:    "business",
				Code:    "LOGIN_FAIL",
				Message: fmt.Sprintf("登录失败，HTTP %d: %s", resp.StatusCode, resp.Text()),
				API:     "/api/auth/login",
			}
		}

		token, ok := resp.JSON("data.token").(string)
		if !ok || token == "" {
			return fmt.Errorf("登录响应中未找到 token")
		}
		ctx.Vars.Set("token", token)
		
		ctx.Log.Debug("VU[%d] 登录成功，token 已缓存", ctx.VUId)
	}

	token := ctx.Vars.GetString("token")

	// 查询当前用户信息
	meResp, err := ctx.HTTP.Get(
		sd.BaseURL+"/api/auth/me",
		sdkhttp.WithHeader("Authorization", "Bearer "+token),
		sdkhttp.WithName("/api/auth/me"),
	)
	if err != nil {
		return err
	}

	ctx.Check.That(meResp).Status(200).RTLt(500)
	return nil
}

func (s *HttpLoginScript) Teardown(ctx *spec.RunContext, data interface{}) error {
	ctx.Log.Info("Teardown: 压测完成")
	return nil
}


