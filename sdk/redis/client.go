// Package redis 提供压测脚本使用的 Redis 客户端。
// 自动调用 ctx.Recorder 上报每次操作的耗时和结果，label 格式为 "redis.CMD"。
//
// 典型用法：
//
//	// Setup 中创建连接池（所有 VU 共享）
//	pool, err := redis.NewPool(ctx, ctx.Vars.Env("REDIS_ADDR"))
//
//	// Default 中执行操作
//	val, err := redis.Get(ctx, pool, "key")
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/A0dongq1N/jarvan4-platform/spec"
	goredis "github.com/redis/go-redis/v9"
)

// Pool Redis 连接池，在 Setup 中创建，所有 VU 共享。
type Pool struct {
	client *goredis.Client
}

// PoolOption 连接池选项函数。
type PoolOption func(*goredis.Options)

// WithPassword 设置认证密码。
func WithPassword(password string) PoolOption {
	return func(o *goredis.Options) { o.Password = password }
}

// WithDB 选择数据库编号（默认 0）。
func WithDB(db int) PoolOption {
	return func(o *goredis.Options) { o.DB = db }
}

// WithPoolSize 设置连接池大小（默认 10 * CPU 核心数）。
func WithPoolSize(size int) PoolOption {
	return func(o *goredis.Options) { o.PoolSize = size }
}

// NewPool 创建 Redis 连接池。
// addr 格式：host:port，如 "localhost:6379"。
func NewPool(ctx *spec.RunContext, addr string, opts ...PoolOption) (*Pool, error) {
	o := &goredis.Options{
		Addr:         addr,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}
	for _, opt := range opts {
		opt(o)
	}
	client := goredis.NewClient(o)
	// 验证连通性
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis ping %s: %w", addr, err)
	}
	return &Pool{client: client}, nil
}

// Close 关闭连接池，在 Teardown 中调用。
func (p *Pool) Close() error {
	return p.client.Close()
}

// Get 获取字符串值，自动上报 "redis.GET" 指标。
// key 不存在时返回 ("", nil)。
func Get(ctx *spec.RunContext, pool *Pool, key string) (string, error) {
	start := time.Now()
	val, err := pool.client.Get(context.Background(), key).Result()
	duration := time.Since(start)

	if err == goredis.Nil {
		record(ctx, "redis.GET", duration, nil)
		return "", nil
	}
	record(ctx, "redis.GET", duration, err)
	return val, err
}

// Set 设置字符串值，ttl=0 表示不过期，自动上报 "redis.SET" 指标。
func Set(ctx *spec.RunContext, pool *Pool, key string, value interface{}, ttl time.Duration) error {
	start := time.Now()
	err := pool.client.Set(context.Background(), key, value, ttl).Err()
	record(ctx, "redis.SET", time.Since(start), err)
	return err
}

// Del 删除一个或多个 key，自动上报 "redis.DEL" 指标。
func Del(ctx *spec.RunContext, pool *Pool, keys ...string) error {
	start := time.Now()
	err := pool.client.Del(context.Background(), keys...).Err()
	record(ctx, "redis.DEL", time.Since(start), err)
	return err
}

// HGet 获取 hash field 值，自动上报 "redis.HGET" 指标。
func HGet(ctx *spec.RunContext, pool *Pool, key, field string) (string, error) {
	start := time.Now()
	val, err := pool.client.HGet(context.Background(), key, field).Result()
	duration := time.Since(start)
	if err == goredis.Nil {
		record(ctx, "redis.HGET", duration, nil)
		return "", nil
	}
	record(ctx, "redis.HGET", duration, err)
	return val, err
}

// HSet 设置 hash field 值，values 格式为 field1, value1, field2, value2...，自动上报 "redis.HSET" 指标。
func HSet(ctx *spec.RunContext, pool *Pool, key string, values ...interface{}) error {
	start := time.Now()
	err := pool.client.HSet(context.Background(), key, values...).Err()
	record(ctx, "redis.HSET", time.Since(start), err)
	return err
}

// Incr 对 key 的值加 1，自动上报 "redis.INCR" 指标。
func Incr(ctx *spec.RunContext, pool *Pool, key string) (int64, error) {
	start := time.Now()
	val, err := pool.client.Incr(context.Background(), key).Result()
	record(ctx, "redis.INCR", time.Since(start), err)
	return val, err
}

// Expire 设置 key 过期时间，自动上报 "redis.EXPIRE" 指标。
func Expire(ctx *spec.RunContext, pool *Pool, key string, ttl time.Duration) error {
	start := time.Now()
	err := pool.client.Expire(context.Background(), key, ttl).Err()
	record(ctx, "redis.EXPIRE", time.Since(start), err)
	return err
}

func record(ctx *spec.RunContext, label string, duration time.Duration, err error) {
	if ctx != nil && ctx.Recorder != nil {
		ctx.Recorder.Record(label, duration, err)
	}
}
