# jarvan4-script — 压测脚本仓库

独立 Git 仓库，存放压测脚本源码。CI 自动编译为 Go plugin（`.so`）后上传到对象存储，Worker 从对象存储拉取并加载执行。

## 模块路径

```
module github.com/A0dongq1N/jarvan4-script
```

依赖 `github.com/A0dongq1N/jarvan4-platform`（契约在 `spec/`，HTTP 选项为 `spec.WithName` / `WithHeader` / `WithQuery`）。本地开发由仓库根目录 `go.work` 指向 `../jarvan4-platform`，不必在本模块 `go.mod` 里写 `replace`。其它协议 SDK 在本仓库 `sdk/`（如 `sdk/tcp`、`sdk/ws`）。

## 目录结构

```
jarvan4-script/
├── scripts/                 # 压测脚本目录(每子目录一个独立脚本)
│   ├── http_demo/           # 示例:简单 HTTP GET 压测
│   │   └── main.go          # 入口(必须导出 var Script spec.ScriptEntry)
│   ├── http_login/          # 示例:登录 + 查询流程压测
│   │   └── main.go
│   └── _target/             # 本地联调目标服务(stdlib HTTP server)
│       └── main.go
├── go.mod
├── go.sum
├── .github/workflows/
│   └── build.yml            # CI:vet + 编译所有脚本 + 上传 COS + 通知 Master
└── README.md
```

## 开发一个新脚本

### 1. 复制模板

```bash
cp -r scripts/http_demo scripts/my_new_script
```

修改 `scripts/my_new_script/main.go`：

```go
//go:build plugin

package main

import (
    "fmt"

    "github.com/A0dongq1N/jarvan4-platform/spec"
    sdkhttp "github.com/A0dongq1N/jarvan4-script/sdk/http"
)

var Script spec.ScriptEntry = &MyScript{}
var PluginABI = spec.PluginABIVersion

type MyScript struct{}

type setupData struct {
    HTTP    *sdkhttp.Client
    BaseURL string
}

// Setup 压测开始前执行一次(全局,非每个 goroutine)
// 返回值会传递给每个 VU 的 Default
func (s *MyScript) Setup(ctx *spec.RunContext) (interface{}, error) {
    baseURL := ctx.Vars.Env("BASE_URL")
    if baseURL == "" {
        return nil, fmt.Errorf("BASE_URL 未配置")
    }
    return &setupData{HTTP: sdkhttp.New(ctx), BaseURL: baseURL}, nil
}

// Default 每个 VU 每次迭代调用一次(压测核心逻辑)
func (s *MyScript) Default(ctx *spec.RunContext) error {
    sd := ctx.SetupData.(*setupData)
    res, err := sd.HTTP.Get(ctx, sd.BaseURL+"/api/endpoint")
    if err != nil {
        return err
    }
    ctx.Check.That(res).Status(200)
    return nil
}

// Teardown 所有 VU 结束后执行一次
func (s *MyScript) Teardown(ctx *spec.RunContext, data interface{}) error {
    if sd, ok := data.(*setupData); ok && sd.HTTP != nil {
        sd.HTTP.Close()
    }
    return nil
}
```

### 2. 脚本约束

- **必须** `//go:build plugin` tag(防止误编译进 main)
- **必须** 导出 `var Script spec.ScriptEntry`(Worker 通过 `plugin.Lookup("Script")` 获取)
- **禁止** 启动独立 goroutine / `os.Exit()`(由引擎调度)
- **只允许** import 标准库 + `github.com/A0dongq1N/jarvan4-platform/spec` + `github.com/A0dongq1N/jarvan4-script/sdk/...`
- **环境变量** 通过 `ctx.Vars.Env("KEY")` 读取(从 Master 平台下发)
- **HTTP 请求** 用 `sdk/http`：Setup 里 `sdkhttp.New(ctx)`，Default 里 `client.Get(ctx, url, ...)`
- **断言** 用 `ctx.Check.That(res).Status(200).BodyContains("...")`,失败计入 fail
- **指标自动记录**: HTTP 4xx/5xx、网络错误都会自动计入 fail,**不要** 重复 `totalReqs++`

### 3. 验证脚本能跑通

```bash
# 1. 启动目标服务
cd scripts/_target && go run . &

# 2. 编译脚本（本地调试用，不上传 COS）
make local-so

# 3. 在 Master 管理台创建任务，脚本选刚编译的版本，BASE_URL 指向 http://localhost:8888
#    Worker 会加载 .so 并对 _target 发压
```

### 4. 提交 + 推送

```bash
git add scripts/my_new_script/
git commit -m "feat: 新增 XXX 压测脚本"
git push origin main
```

CI 会自动:
1. `go vet` 检查语法
2. 编译所有 `scripts/*/main.go` 验证能出 `.so`(本次修复后)
3. 检测改动的脚本,编译为 `dist/{name}/{commit_hash}.so`
4. 上传到 COS + 通知 Master
5. Master 平台 `/api/internal/scripts/publish` 接收,数据库新增版本记录

## SDK 接口速查

契约见 [jarvan4-platform/spec/](../jarvan4-platform/spec/)，协议客户端见本仓库 `sdk/`。最常用:

```go
type ScriptEntry interface {
    Setup(ctx *RunContext) (data interface{}, err error)
    Default(ctx *RunContext) error
    Teardown(ctx *RunContext, data interface{}) error
}

type RunContext struct {
    context.Context
    VUId      int           // 当前虚拟用户 ID(1 开始,Setup/Teardown 时为 0)
    Iteration int64         // 已完成迭代数
    WorkerID  string        // Worker 节点 ID
    SetupData interface{}   // Setup 返回的共享数据（含 HTTP/Redis 客户端）
    Check     Checker       // 断言:Check.That(res).Status(200).RTLt(2000)
    Vars      VarStore      // 变量:Env(key) 读平台变量,Set/Get 私有变量
    Log       Logger        // 日志(上报到 Master 实时看板)
    Sleep     Sleeper       // 睡眠(可被引擎停止信号中断)
    Recorder  MetricsRecorder // 协议 SDK 内部自动调用
}

// HTTP：sdk/http.New(ctx) → client.Get(ctx, url, spec.WithName(...))
// Redis：sdk/redis.NewPool(ctx, addr) → redis.Get(ctx, pool, key)
```

## 测试策略

本项目**不写单元测试、不用 mock**(见 jarvan4-platform/CODEBUDDY.md 「测试策略」铁律)。

脚本正确性通过**真实发压**验证:
- **CI**: `go vet` + 编译所有脚本(确保都能产出 `.so`)
- **本地**: 启 `_target`，`make local-so`，在 Master 管理台创建任务由 Worker 加载执行
- **生产**: 脚本被 Worker 实际加载,对真实被压测服务发起流量

如果发现脚本有 bug,正确的做法是:
1. 启本地 `_target` 复现
2. 创建短时任务，观察 Worker 日志和管理台看板
3. 修复后重新编译并再跑一遍
4. 提交 + 推送

**禁止** 写 mock HTTP server 测试脚本(本项目已删除 `testkit/` 包作为教训)。

## 常见问题

### Q: 脚本名能改吗?
A: ❌ 不能。脚本名(子目录名)与 Master 平台 `script.name` 强绑定。改名会被识别为新脚本,历史执行数据无法关联。

### Q: 编译产物 .so 提交到 git 吗?
A: ❌ 不提交。CI 编译后上传到 COS,Worker 拉取。

### Q: 修改了 SDK 接口,需要做什么?
A: 升级 SDK 版本 → 全量重编译所有脚本(在 GitHub Actions 页面 "Run workflow" 触发 `workflow_dispatch`)。

### Q: 如何调试脚本?
A: 编译后通过 Master 创建短时任务（VU=1、duration 几秒），看 Worker 日志和管理台实时看板里的 panic / 错误 / 状态。

### Q: 脚本抛 panic 怎么办?
A: SDK 内部会 recover 算作 fail。但应避免 panic,改用显式 error 返回:
```go
if err != nil {
    return err  // SDK 计入 fail
}
```

## CI

`.github/workflows/build.yml` 在 push 到 main 时:
1. `go vet` 检查
2. 编译所有 `scripts/*/main.go` 验证能出 `.so`
3. 检测本次改动的脚本
4. 编译为 `dist/{name}/{commit_hash}.so`
5. 上传到 COS
6. 通知 Master API

## 相关仓库

- [jarvan4-platform](../jarvan4-platform) — Master + Worker + SDK
- [jarvan4-web](../jarvan4-web) — 前端管理台