//go:build plugin

package main

import (
	"testing"

	"stress-scripts/testkit"
)

func TestSetup_MissingBaseURL(t *testing.T) {
	ctx := testkit.NewContext() // 不设置 BASE_URL
	s := &HttpLoginScript{}
	_, err := s.Setup(ctx)
	if err == nil {
		t.Fatal("期望 BASE_URL 未配置时返回 error")
	}
}

func TestSetup_Success(t *testing.T) {
	ctx := testkit.NewContext(testkit.WithEnv("BASE_URL", "http://example.com"))
	s := &HttpLoginScript{}
	data, err := s.Setup(ctx)
	if err != nil {
		t.Fatalf("Setup 不应报错: %v", err)
	}
	sd := data.(*setupData)
	if sd.BaseURL != "http://example.com" {
		t.Errorf("BaseURL 不匹配，期望 http://example.com 实际 %s", sd.BaseURL)
	}
}

func TestDefault_LoginAndQuery(t *testing.T) {
	mock := testkit.NewMockHTTP().
		On("http://example.com/api/auth/login",
			testkit.OK([]byte(`{"code":0,"data":{"token":"test-token-123"}}`))).
		On("http://example.com/api/auth/me",
			testkit.OK([]byte(`{"code":0,"data":{"id":"admin-001","username":"admin"}}`)))

	ctx := testkit.NewContext(
		testkit.WithHTTP(mock),
		testkit.WithEnv("BASE_URL", "http://example.com"),
		testkit.WithVUId(1),
		testkit.WithSetupData(&setupData{BaseURL: "http://example.com"}),
	)

	s := &HttpLoginScript{}
	err := s.Default(ctx)
	if err != nil {
		t.Fatalf("Default 不应报错: %v", err)
	}

	// 验证发出了 2 个请求
	if mock.RequestCount() != 2 {
		t.Errorf("期望 2 个请求，实际 %d", mock.RequestCount())
	}
	// 验证 token 被正确缓存
	if ctx.Vars.GetString("token") != "test-token-123" {
		t.Errorf("token 缓存错误: %s", ctx.Vars.GetString("token"))
	}
}

func TestDefault_TokenReuse(t *testing.T) {
	mock := testkit.NewMockHTTP().
		On("http://example.com/api/auth/me",
			testkit.OK([]byte(`{"code":0,"data":{"id":"admin-001"}}`)))

	ctx := testkit.NewContext(
		testkit.WithHTTP(mock),
		testkit.WithSetupData(&setupData{BaseURL: "http://example.com"}),
		testkit.WithVUId(1),
	)
	// 预先设置 token，模拟第二次迭代
	ctx.Vars.Set("token", "existing-token")

	s := &HttpLoginScript{}
	err := s.Default(ctx)
	if err != nil {
		t.Fatalf("Default 不应报错: %v", err)
	}

	// 第二次迭代不应重新登录，只发 1 个请求
	if mock.RequestCount() != 1 {
		t.Errorf("token 复用失败，期望 1 个请求，实际 %d", mock.RequestCount())
	}
	if mock.RequestAt(0).URL != "http://example.com/api/auth/me" {
		t.Errorf("期望请求 /api/auth/me，实际 %s", mock.RequestAt(0).URL)
	}
}

func TestDefault_LoginFail(t *testing.T) {
	mock := testkit.NewMockHTTP().
		On("http://example.com/api/auth/login", testkit.Status(401))

	ctx := testkit.NewContext(
		testkit.WithHTTP(mock),
		testkit.WithSetupData(&setupData{BaseURL: "http://example.com"}),
	)

	s := &HttpLoginScript{}
	err := s.Default(ctx)
	if err == nil {
		t.Fatal("期望登录失败时返回 error")
	}
}

func TestDefault_NoNetworkPanic(t *testing.T) {
	mock := testkit.NewMockHTTP() // 无预设响应，全部返回 200 空 body

	ctx := testkit.NewContext(
		testkit.WithHTTP(mock),
		testkit.WithSetupData(&setupData{BaseURL: "http://example.com"}),
	)

	s := &HttpLoginScript{}
	// 任何情况下脚本都不应 panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("脚本 panic: %v", r)
		}
	}()
	s.Default(ctx) //nolint:errcheck
}
