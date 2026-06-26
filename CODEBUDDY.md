# CODEBUDDY.md — jarvan4-script

压测脚本仓库。每个脚本编译为 Go plugin（`.so`），由 Worker 动态加载执行。

模块路径：`stress-scripts`（独立 Git 仓库）

## 脚本编写规范

### 必须遵守的约束

1. **`package main`**，必须导出 `var Script spec.ScriptEntry`
2. **第一行必须是** `//go:build plugin`
3. **只允许 import**：标准库 + `github.com/Aodongq1n/jarvan4-platform/sdk`
4. **禁止**：启动独立 goroutine、`os.Exit()`、直接读写文件
5. **脚本名**（子目录名）一旦确定不可修改（与平台 `script.name` 绑定）

### ScriptEntry 接口

```go
type ScriptEntry interface {
    // Setup 压测前执行一次（全局），返回 setupData 传给所有 VU
    Setup(ctx *RunContext) (data interface{}, err error)
    // Default 每次迭代调用，压测核心主体
    Default(ctx *RunContext) error
    // Teardown 压测结束后执行一次
    Teardown(ctx *RunContext, data interface{}) error
}
```

### RunContext 能力

| 字段 | 用途 |
|------|------|
| `ctx.VUId` | 当前虚拟用户编号（1 开始） |
| `ctx.Iteration` | 本 VU 已完成迭代数 |
| `ctx.HTTP` | HTTP 客户端，自动上报指标（`ctx.HTTP.Get/Post/...`） |
| `ctx.Check` | 断言：`ctx.Check.That(resp).Status(200).RTLt(2000)` |
| `ctx.Vars` | 环境变量：`ctx.Vars.Env("BASE_URL")` |
| `ctx.Log` | 日志，输出到 Master 实时看板 |
| `ctx.Sleep` | 睡眠，可被引擎停止信号中断 |
| `ctx.Recorder` | 协议无关指标记录器（通常无需直接使用） |

### 脚本示例

```go
//go:build plugin

package main

import (
    "fmt"
    sdkhttp "github.com/Aodongq1n/jarvan4-platform/sdk/http"
    "github.com/Aodongq1n/jarvan4-platform/sdk/spec"
)

var Script spec.ScriptEntry = &MyScript{}

type MyScript struct{}

func (s *MyScript) Setup(ctx *spec.RunContext) (interface{}, error) {
    baseURL := ctx.Vars.Env("BASE_URL")
    if baseURL == "" {
        return nil, fmt.Errorf("BASE_URL 环境变量未配置")
    }
    return nil, nil
}

func (s *MyScript) Default(ctx *spec.RunContext) error {
    baseURL := ctx.Vars.Env("BASE_URL")
    res, err := ctx.HTTP.Get(baseURL + "/api/health")
    if err != nil {
        return err
    }
    ctx.Check.That(res).Status(200).RTLt(2000)
    return nil
}

func (s *MyScript) Teardown(ctx *spec.RunContext, data interface{}) error {
    return nil
}
```

## 编译

```bash
# 单个脚本
go build -buildmode=plugin -o dist/http_demo.so ./scripts/http_demo/

# 所有脚本
for d in scripts/*/; do
    name=$(basename "$d")
    go build -buildmode=plugin -o "dist/${name}.so" "./scripts/${name}/"
done
```

**编译环境一致性要求**：CI 必须使用与 Worker 部署相同的 Docker 镜像编译，否则 `plugin.Open` 会报版本不匹配错误。

## 目录结构

```
jarvan4-script/
├── scripts/
│   ├── http_demo/        # HTTP GET 压测示例
│   │   └── main.go
│   ├── http_login/       # 登录 + 查询流程示例
│   │   └── main.go
│   ├── _panic_test/      # panic 测试脚本（下划线前缀不发布）
│   ├── _target/          # 本地测试目标服务
│   └── _test/             # 测试辅助
├── go.mod
└── README.md
```

## CI 流程

```
git push → CI 检测变更脚本 → go vet + 单测 → go build -buildmode=plugin
→ 上传 .so 到 COS → 通知 Master 新版本可用（POST /api/internal/scripts/publish）
```

脚本发布接口（Master 端）：
```
POST /api/internal/scripts/publish
{
  "projectId": "...",
  "name": "http_demo",
  "commitHash": "abc123",
  "artifactUrl": "scripts/http_demo/abc123.so",
  "commitMsg": "...",
  "author": "..."
}
```
