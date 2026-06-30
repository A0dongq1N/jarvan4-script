# 被测服务（Target）

最小 HTTP 服务，供本地开发和 E2E 测试压测。真实部署时被测对象是用户的业务服务，本服务仅用于：

1. **本地开发** — 验证压测脚本能跑通
2. **CI E2E** — 测试框架拉起本服务，跑端到端压测
3. **教学** — 作为 SDK HTTP API 的最小可用示例

## 启动

```bash
# 编译（需要去掉 //go:build ignore 标签，或测试框架会自动处理）
go build -o target_bin .

# 运行
TARGET_ADDR=:8888 ./target_bin
```

环境变量：
- `TARGET_ADDR`（可选）— 监听地址，默认 `:8888`

## 接口

| 端点 | 方法 | 说明 | 参数 |
|------|------|------|------|
| `/get` | GET | 返回 200 + echo query 参数（JSON） | query 任意 |
| `/api/auth/login` | POST | 模拟登录，任意账号返回 token | body: `{"username":"...","password":"..."}` |
| `/api/auth/me` | GET | 校验 Authorization header | header: `Authorization: Bearer xxx` |
| `/api/slow` | GET | 故意 sleep，压响应时间 | query: `sleep_ms`（默认 50） |
| `/api/error` | GET | 按概率返回 500，压错误率 | query: `rate`（默认 0.3） |
| `/__stats` | GET | 返回各端点累计调用次数（仅测试用） | 无 |

## 响应示例

### GET /get?key=value

```json
{
  "args": {"key": "value"},
  "url": "/get?key=value",
  "headers": {...},
  "origin": "127.0.0.1:xxx"
}
```

### POST /api/auth/login

```json
{
  "code": 0,
  "data": {
    "token": "test-token-1782703047366158860",
    "username": "admin"
  }
}
```

### GET /__stats

```json
{
  "login": 100,
  "me": 50,
  "slow": 30,
  "error": 20,
  "success": 180
}
```

## 在 E2E 测试中的使用

测试框架（`master/integration_test/worker_full_setup_test.go`）会：
1. 编译 `target_bin`（从本源码去掉 `//go:build ignore` 标签）
2. 启动 target 服务在 `:8888`
3. 压测脚本通过 `BASE_URL=http://127.0.0.1:8888` 访问
4. 测试结束后通过 `/__stats` 验证请求是否真正到达被测服务
