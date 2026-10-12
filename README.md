# jarvan4-script — 压测脚本仓库

独立 Git 仓库，存放压测脚本源码。每个脚本编译为**独立可执行文件**，CI 上传到 COS；Worker 下载后 `exec` 执行（引擎在 `jarvan4-platform/scriptrun` 内，不再使用 Go plugin / `.so`）。

## 模块路径

```
module github.com/A0dongq1N/jarvan4-script
```

依赖 `github.com/A0dongq1N/jarvan4-platform`（契约在 `spec/`，运行时入口在 `scriptrun/`）。本地开发由三个子项目共同工作区根目录的 `go.work` 引用 `./jarvan4-platform`、`./jarvan4-platform/pb` 和 `./jarvan4-script`，不必在本模块 `go.mod` 里写 `replace`。协议客户端在本仓库 `client/`（如 `client/http`、`client/redis`），工具在 `util/`。

## 目录结构

```
jarvan4-script/
├── scripts/                 # 压测脚本（每子目录一个独立二进制）
│   ├── http_get_echo/           # GET /get（echo query）
│   │   └── main.go          # package main + scriptrun.Main(...)
│   ├── http_auth_login_me/      # POST login + GET /api/auth/me
│   ├── http_get_error_rate/     # GET /api/error（ERROR_RATE，熔断联调）
│   ├── redis_set_get/           # Redis SET + GET 读写校验
│   ├── _panic_test/         # panic 测试（下划线前缀，不发布）
│   └── _target/             # 本地联调目标服务
├── client/                  # 协议客户端（http / redis / …，自动 ctx.Recorder 上报）
├── util/                    # 脚本工具（random.go：Int/String 等）
├── Makefile
├── go.mod
├── .github/workflows/
│   └── build.yml            # CI：vet + 编译 + 上传 COS + 通知 Master
└── README.md
```

## 开发一个新脚本

### 1. 复制模板

```bash
cp -r scripts/http_get_echo scripts/my_new_script
```

修改 `scripts/my_new_script/main.go`：

```go
package main

import (
	"fmt"

	"github.com/A0dongq1N/jarvan4-platform/scriptrun"
	"github.com/A0dongq1N/jarvan4-platform/spec"
	sdkhttp "github.com/A0dongq1N/jarvan4-script/client/http"
)

func main() {
	scriptrun.Main(&MyScript{})
}

type MyScript struct{}

type setupData struct {
	HTTP    *sdkhttp.Client
	BaseURL string
}

// Setup 压测开始前执行一次（全局，非每个 goroutine）
func (s *MyScript) Setup(ctx *spec.RunContext) (interface{}, error) {
	baseURL := ctx.Vars.Env("BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("BASE_URL 未配置")
	}
	return &setupData{HTTP: sdkhttp.New(ctx), BaseURL: baseURL}, nil
}

// Default 每个 VU 每次迭代调用一次（压测核心逻辑）
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

- **必须** `package main`，入口 `func main() { scriptrun.Main(&Xxx{}) }`
- **必须** 实现 `spec.ScriptEntry`（Setup / Default / Teardown）
- **禁止** 启动独立 goroutine / `os.Exit()`（由引擎调度）
- **只允许** import：标准库 + `jarvan4-platform/spec` + `jarvan4-platform/scriptrun` + 本仓库 `client/...` 与 `util/...`
- **环境变量** 通过 `ctx.Vars.Env("KEY")` 读取（Master 平台下发）
- **HTTP** 用 `client/http`：Setup 里 `sdkhttp.New(ctx)`，Default 里 `client.Get(ctx, url, ...)`
- **Redis** 用 `client/redis`：Setup 建池，经 SetupData 共享
- **断言** 用 `ctx.Check.That(res).Status(200).BodyContains("...")`，失败计入 fail
- **指标自动记录**：协议错误会自动计入 fail，**不要** 手动累加请求数

### 3. 本地编译

```bash
make build-bin    # 编译所有正式脚本二进制 → dist/
```

上传和通知 Master 只走 CI：推送分支得到预览版，推送 `main` 得到正式版。

本地联调：

```bash
# 1. 启动目标服务
cd scripts/_target && go run . &

# 2. 编译脚本
make build-bin

# 3. 在 Master 管理台创建任务，BASE_URL 指向 http://localhost:8888
#    Worker 会下载/exec 二进制并对 _target 发压
```

### 4. 提交 + 推送

```bash
git add scripts/my_new_script/
git commit -m "feat: 新增 XXX 压测脚本"
git push origin main
```

CI 会自动：

1. `go vet`
2. 编译所有正式脚本（跳过 `_` 前缀目录）做 sanity check
3. 检测本次改动的脚本，编译为 `dist/{name}/{commitHash}`
4. 上传到 COS（版本键 + latest 键）
5. 通知 Master `POST /api/internal/scripts/publish`，写入新版本记录

`artifactUrl` 形如 `scripts/http_get_echo/http_get_echo`（无 `.so` 后缀）。

## SDK 接口速查

契约见 [jarvan4-platform/spec/](../jarvan4-platform/spec/)，运行时见 [scriptrun/](../jarvan4-platform/scriptrun/)，协议客户端见本仓库 `client/`。

```go
type ScriptEntry interface {
	Setup(ctx *RunContext) (data interface{}, err error)
	Default(ctx *RunContext) error
	Teardown(ctx *RunContext, data interface{}) error
}

type RunContext struct {
	context.Context
	VUId      int
	Iteration int64
	WorkerID  string
	SetupData interface{}
	Check     Checker
	Vars      VarStore
	Log       Logger
	Sleep     Sleeper
	Recorder  MetricsRecorder
}

// HTTP：client/http.New(ctx) → client.Get(ctx, url, spec.WithName(...))
// Redis：client/redis.NewPool(ctx, addr) → redis.Get(ctx, pool, key)
```

CLI（Worker 调用，脚本作者一般不用手敲）：

| 参数 | 作用 |
|------|------|
| `--self-check` | 校验可执行后 exit 0（Prepare 阶段） |
| `--run-json <path>` | 读 Worker 下发的压测配置并开跑 |

## 测试策略

本项目**不写单元测试、不用 mock**（见 jarvan4-platform/CODEBUDDY.md「测试铁律」）。

脚本正确性通过**真实发压**验证：

- **CI**：`go vet` + 编译所有正式脚本
- **本地**：启 `_target`，`make build-bin`，在 Master 创建短时任务由 Worker exec 执行
- **生产**：Worker 下载二进制，对真实被压测服务发压

若脚本有 bug：

1. 启本地 `_target` 复现
2. 创建短时任务（VU=1、几秒），看 Worker / 脚本进程日志与看板
3. 修复后 push，由 CI 发布后再跑一遍

**禁止** 写 mock HTTP server 测脚本。

## 常见问题

### Q: 脚本名能改吗？

A: ❌ 不能。子目录名与 Master `script.name` 强绑定；改名会被当成新脚本。

### Q: 编译产物提交到 git 吗？

A: ❌ 不提交。CI 上传到 COS，Worker 按 commitHash 拉取缓存。

### Q: 还需要 `//go:build plugin` / `var Script` / `PluginABI` 吗？

A: ❌ 已废弃。入口改为 `scriptrun.Main`，产物是普通二进制，不再有 ABI 指纹对齐问题。

### Q: 修改了 platform/scriptrun 或本仓库 SDK，需要做什么？

A: 推送脚本仓（或在 GitHub Actions 用 `workflow_dispatch` 全量重编）。CI 会 clone 最新 `jarvan4-platform` main 再编译。修改 `client/` 或 `util/` 后同样需要重新编译上传脚本。

### Q: 如何调试脚本？

A: `make build-bin` 后通过 Master 建短时任务，看 Worker 日志与管理台实时看板里的 panic / 错误。也可手动：

```bash
./dist/http_get_echo --self-check
./dist/http_get_echo --run-json /path/to/run.json
```

### Q: 脚本抛 panic 怎么办？

A: 引擎会 recover 并计 fail。应尽量显式返回 error：

```go
if err != nil {
	return err
}
```

## CI

`.github/workflows/build.yml` 在 push `main`（改动 `scripts/**` / `go.mod` / `go.sum`）或手动触发时：

1. Clone `jarvan4-platform`（含 `scriptrun` / `pb`）并 `replace`
2. `go vet` + 全量编译 sanity check
3. 编译变更脚本为普通二进制
4. 上传 COS + 通知 Master

## 相关仓库

- [jarvan4-platform](../jarvan4-platform) — Master + Worker + `spec` / `scriptrun`
- [jarvan4-web](../jarvan4-web) — 前端管理台
- 脚本体系：[doc/design/04-脚本体系.md](../jarvan4-platform/doc/design/04-脚本体系.md)
- 独立二进制决策：[ADR-0001](../jarvan4-platform/doc/adr/0001-脚本使用独立二进制.md)
