# CODEBUDDY.md — jarvan4-script

压测脚本仓库。每个脚本编译为**独立可执行文件**，由 Worker 下载后 exec 执行（引擎在 `scriptrun` 内）。

模块路径：`github.com/A0dongq1N/jarvan4-script`（独立 Git 仓库）

## 脚本编写规范

### 必须遵守的约束

1. **`package main`**，`func main() { scriptrun.Main(&XxxScript{}) }`
2. **实现** `spec.ScriptEntry`（Setup / Default / Teardown）
3. **只允许 import**：标准库 + `jarvan4-platform/spec` + `jarvan4-platform/scriptrun` + 本仓库 `sdk/`
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
| `ctx.Check` | 断言：`ctx.Check.That(resp).Status(200).RTLt(2000)` |
| `ctx.Vars` | 环境变量：`ctx.Vars.Env("BASE_URL")` |
| `ctx.Log` | 日志，输出到 Master 实时看板 |
| `ctx.Sleep` | 睡眠，可被引擎停止信号中断 |
| `ctx.Recorder` | 协议无关指标记录器（通常无需直接使用） |
| `ctx.SetupData` | Setup 返回值（放共享连接池 / HTTP Client） |

协议客户端不在 Context 上：HTTP 用 `sdk/http`，Redis 用 `sdk/redis`，均在 Setup 创建、经 SetupData 共享。

### 脚本示例

```go
package main

import (
    "fmt"
    "github.com/A0dongq1N/jarvan4-platform/scriptrun"
    "github.com/A0dongq1N/jarvan4-platform/spec"
    sdkhttp "github.com/A0dongq1N/jarvan4-script/sdk/http"
)

func main() { scriptrun.Main(&MyScript{}) }

type MyScript struct{}

type setupData struct {
    HTTP    *sdkhttp.Client
    BaseURL string
}

func (s *MyScript) Setup(ctx *spec.RunContext) (interface{}, error) {
    baseURL := ctx.Vars.Env("BASE_URL")
    if baseURL == "" {
        return nil, fmt.Errorf("BASE_URL 环境变量未配置")
    }
    return &setupData{HTTP: sdkhttp.New(ctx), BaseURL: baseURL}, nil
}

func (s *MyScript) Default(ctx *spec.RunContext) error {
    sd := ctx.SetupData.(*setupData)
    res, err := sd.HTTP.Get(ctx, sd.BaseURL+"/api/health")
    if err != nil {
        return err
    }
    ctx.Check.That(res).Status(200).RTLt(2000)
    return nil
}

func (s *MyScript) Teardown(ctx *spec.RunContext, data interface{}) error {
    if sd, ok := data.(*setupData); ok && sd.HTTP != nil {
        sd.HTTP.Close()
    }
    return nil
}
```

## 编译 & 上传

**统一用 Makefile target，不要手敲 go build 命令：**

```bash
make build-bin    # 编译所有正式脚本二进制
make upload-bin   # 编译 + 上传到 COS + 通知 Master
make local-bin    # 仅编译不上传
make env-check
make help
```

首次配置 COS 密钥（写入 ~/.bashrc 永久生效）：
```bash
echo 'export COS_SECRET_ID=xxx' >> ~/.bashrc
echo 'export COS_SECRET_KEY=xxx' >> ~/.bashrc
source ~/.bashrc
```

Makefile 位置：`jarvan4-script/Makefile`。

本地开发依赖根目录 `go.work`（含 `jarvan4-platform` 与 `jarvan4-platform/pb`）。CI clone platform 并用 replace（含 pb 子模块）。旧 `make *-so` 仍是兼容别名。

## 目录结构

```
jarvan4-script/
├── scripts/
│   ├── http_demo/        # HTTP GET 压测示例
│   │   └── main.go
│   ├── http_login/       # 登录 + 查询流程示例
│   │   └── main.go
│   ├── redis_demo/       # Redis SET + GET 读写压测
│   │   └── main.go
│   ├── _panic_test/      # panic 测试脚本（下划线前缀不发布）
│   └── _target/          # 本地测试目标服务
├── go.mod
└── README.md
```

### redis_demo 环境变量

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `REDIS_ADDR` | 是 | - | `host:port`，如 `127.0.0.1:6379` |
| `REDIS_PASSWORD` | 否 | 空 | Redis 认证密码 |
| `REDIS_DB` | 否 | `0` | 数据库编号 |
| `KEY_PREFIX` | 否 | `jarvan4:stress:` | key 前缀，避免污染业务数据 |
| `TTL_SECONDS` | 否 | `300` | SET 过期秒数 |
| `VALUE_SIZE` | 否 | `64` | value 字节长度 |

每轮迭代：`SET {prefix}{workerId}:{vuId}:{iteration}` → `GET` 校验返回值。`workerId` 用于避免多 Worker 本地 `VUId` 撞 key。指标维度：`redis.SET`、`redis.GET`。

## CI 流程

```
git push → CI 检测变更脚本 → go vet → go build（普通二进制）
→ 上传到 COS → 通知 Master（POST /api/internal/scripts/publish）
```

脚本发布接口（Master 端）：
```
POST /api/internal/scripts/publish
{
  "projectId": "...",
  "name": "http_demo",
  "commitHash": "abc123",
  "artifactUrl": "scripts/http_demo/http_demo",
  "commitMsg": "...",
  "author": "..."
}
```
