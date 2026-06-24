//go:build ignore

// Package main 启动一个最小的 HTTP target 服务,供本地 e2e 压测。
// 真实部署时被压测的是用户的业务服务,这个 _target 仅用于:
//   1. 本地开发:验证压测脚本能跑通
//   2. CI e2e:scripts/_test/e2e_test.sh 拉起本服务,跑 cmd/runner
//   3. 教学:作为 SDK HTTP API 的最小可用示例
//
// 端点:
//   GET  /get       → 200 + echo query 参数(JSON)
//   POST /api/auth/login → 登录(成功返回 token)
//   GET  /api/auth/me    → 校验 Authorization header
//   GET  /api/slow       → 故意 sleep 50ms,用来压 RT
//   GET  /api/error      → 30% 概率返回 500
package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

var (
	loginCounter   atomic.Int64
	meCounter      atomic.Int64
	slowCounter    atomic.Int64
	errorCounter   atomic.Int64
	successCounter atomic.Int64
)

func main() {
	addr := os.Getenv("TARGET_ADDR")
	if addr == "" {
		addr = ":8888"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/get", handleGet)
	mux.HandleFunc("/api/auth/login", handleLogin)
	mux.HandleFunc("/api/auth/me", handleMe)
	mux.HandleFunc("/api/slow", handleSlow)
	mux.HandleFunc("/api/error", handleError)
	mux.HandleFunc("/__stats", handleStats) // 仅 e2e 测试用

	fmt.Printf("[_target] listening on %s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}
}

// handleGet GET /get?key=value → {"args": {"key": "value"}}
// 类比 httpbin.org/get
func handleGet(w http.ResponseWriter, r *http.Request) {
	args := map[string]string{}
	for k, vs := range r.URL.Query() {
		if len(vs) > 0 {
			args[k] = vs[0]
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"args":    args,
		"url":     r.URL.String(),
		"headers": flattenHeaders(r.Header),
		"origin":  r.RemoteAddr,
	})
	successCounter.Add(1)
}

// handleLogin POST /api/auth/login
// body: {"username": "...", "password": "..."}
// 任意账号都返回固定 token(用于压测,不做真实鉴权)
func handleLogin(w http.ResponseWriter, r *http.Request) {
	loginCounter.Add(1)
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Username == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing username/password"})
		errorCounter.Add(1)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": map[string]string{
			"token":    fmt.Sprintf("test-token-%d", time.Now().UnixNano()),
			"username": body.Username,
		},
	})
	successCounter.Add(1)
}

// handleMe GET /api/auth/me
// 需要 Authorization: Bearer xxx header
func handleMe(w http.ResponseWriter, r *http.Request) {
	meCounter.Add(1)
	auth := r.Header.Get("Authorization")
	if auth == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing Authorization"})
		errorCounter.Add(1)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": map[string]string{
			"id":       "user-1",
			"username": "test-user",
		},
	})
	successCounter.Add(1)
}

// handleSlow GET /api/slow?sleep_ms=50
// 故意 sleep 用来压 RT
func handleSlow(w http.ResponseWriter, r *http.Request) {
	slowCounter.Add(1)
	sleepMS, _ := strconv.Atoi(r.URL.Query().Get("sleep_ms"))
	if sleepMS <= 0 {
		sleepMS = 50
	}
	time.Sleep(time.Duration(sleepMS) * time.Millisecond)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true", "slept_ms": strconv.Itoa(sleepMS)})
	successCounter.Add(1)
}

// handleError GET /api/error?rate=0.3
// 按概率返回 500,用于压错误率
func handleError(w http.ResponseWriter, r *http.Request) {
	errorCounter.Add(1)
	rate, _ := strconv.ParseFloat(r.URL.Query().Get("rate"), 64)
	if rate <= 0 {
		rate = 0.3
	}
	if rand.Float64() < rate {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	successCounter.Add(1)
}

// handleStats GET /__stats
// e2e 测试用,返回每个端点的累计调用次数
func handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]int64{
		"login":   loginCounter.Load(),
		"me":      meCounter.Load(),
		"slow":    slowCounter.Load(),
		"error":   errorCounter.Load(),
		"success": successCounter.Load(),
	})
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}