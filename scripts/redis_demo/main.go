//go:build plugin

// 压测脚本：Redis SET + GET 读写链路
// 环境变量：
//   REDIS_ADDR     Redis 地址，如 127.0.0.1:6379（必填）
//   REDIS_PASSWORD 认证密码（可选）
//   REDIS_DB       数据库编号（可选，默认 0）
//   KEY_PREFIX     key 前缀（可选，默认 jarvan4:stress:）
//   TTL_SECONDS    SET 过期秒数（可选，默认 300）
//   VALUE_SIZE     value 字节长度（可选，默认 64）
package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/A0dongq1N/jarvan4-platform/spec"
	sdkredis "github.com/A0dongq1N/jarvan4-script/sdk/redis"
	"github.com/A0dongq1N/jarvan4-script/sdk/random"
)

// Script 导出符号，Worker 通过 plugin.Lookup("Script") 获取。
var Script spec.ScriptEntry = &RedisDemoScript{}

// PluginABI 插件 ABI 版本，须与 Worker 内 spec.PluginABIVersion 一致。
var PluginABI = spec.PluginABIVersion

type RedisDemoScript struct{}

type setupData struct {
	Pool      *sdkredis.Pool
	KeyPrefix string
	TTL       time.Duration
	ValueSize int
}

func (s *RedisDemoScript) Setup(ctx *spec.RunContext) (interface{}, error) {
	addr := ctx.Vars.Env("REDIS_ADDR")
	if addr == "" {
		return nil, fmt.Errorf("REDIS_ADDR 环境变量未配置")
	}

	var opts []sdkredis.PoolOption
	if password := ctx.Vars.Env("REDIS_PASSWORD"); password != "" {
		opts = append(opts, sdkredis.WithPassword(password))
	}
	if dbStr := ctx.Vars.Env("REDIS_DB"); dbStr != "" {
		db, err := strconv.Atoi(dbStr)
		if err != nil {
			return nil, fmt.Errorf("REDIS_DB 无效: %w", err)
		}
		opts = append(opts, sdkredis.WithDB(db))
	}

	pool, err := sdkredis.NewPool(ctx, addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("redis 连接失败: %w", err)
	}

	keyPrefix := ctx.Vars.Env("KEY_PREFIX")
	if keyPrefix == "" {
		keyPrefix = "jarvan4:stress:"
	}

	ttlSeconds := envInt(ctx, "TTL_SECONDS", 300)
	if ttlSeconds < 0 {
		return nil, fmt.Errorf("TTL_SECONDS 不能为负数")
	}

	valueSize := envInt(ctx, "VALUE_SIZE", 64)
	if valueSize < 1 {
		return nil, fmt.Errorf("VALUE_SIZE 必须大于 0")
	}

	ctx.Log.Info("Setup 完成，Redis=%s keyPrefix=%s ttl=%ds valueSize=%d", addr, keyPrefix, ttlSeconds, valueSize)
	return &setupData{
		Pool:      pool,
		KeyPrefix: keyPrefix,
		TTL:       time.Duration(ttlSeconds) * time.Second,
		ValueSize: valueSize,
	}, nil
}

func (s *RedisDemoScript) Default(ctx *spec.RunContext) error {
	sd := ctx.SetupData.(*setupData)
	// VUId 只在单 Worker 内从 1 递增；多 Worker 必须带上 WorkerID，否则会写同一 key。
	workerID := ctx.WorkerID
	if workerID == "" {
		workerID = "local"
	}
	key := fmt.Sprintf("%s%s:%d:%d", sd.KeyPrefix, workerID, ctx.VUId, ctx.Iteration)
	value := buildValue(sd.ValueSize, ctx.VUId, int(ctx.Iteration))

	if err := sdkredis.Set(ctx, sd.Pool, key, value, sd.TTL); err != nil {
		return err
	}

	got, err := sdkredis.Get(ctx, sd.Pool, key)
	if err != nil {
		return err
	}
	if got != value {
		return fmt.Errorf("GET mismatch: key=%s want=%q got=%q", key, value, got)
	}
	return nil
}

func (s *RedisDemoScript) Teardown(ctx *spec.RunContext, data interface{}) error {
	if data == nil {
		return nil
	}
	sd, ok := data.(*setupData)
	if !ok || sd.Pool == nil {
		return nil
	}
	if err := sd.Pool.Close(); err != nil {
		return fmt.Errorf("关闭 redis 连接池: %w", err)
	}
	ctx.Log.Info("Teardown 完成")
	return nil
}

func envInt(ctx *spec.RunContext, key string, defaultVal int) int {
	raw := ctx.Vars.Env(key)
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return defaultVal
	}
	return v
}

func buildValue(size int, vuID, iteration int) string {
	// 尾部固定包含 vuId 与 iteration，便于校验；前面用随机串填充到指定长度。
	suffix := fmt.Sprintf("-%d-%d", vuID, iteration)
	if len(suffix) >= size {
		return suffix[:size]
	}
	padding := random.String(size - len(suffix))
	return padding + suffix
}
