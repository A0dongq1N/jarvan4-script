// Package random 提供压测脚本使用的随机数据生成工具。
package random

import (
	"math/rand"
	"time"
)

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// Int 返回 [min, max) 范围内的随机整数。
func Int(min, max int) int {
	if min >= max {
		return min
	}
	return min + rng.Intn(max-min)
}

// Int64 返回 [min, max) 范围内的随机 int64。
func Int64(min, max int64) int64 {
	if min >= max {
		return min
	}
	return min + rng.Int63n(max-min)
}

// String 返回指定长度的随机字母数字字符串。
func String(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rng.Intn(len(charset))]
	}
	return string(b)
}

// Pick 从切片中随机选取一个元素。
func Pick[T any](items []T) T {
	return items[rng.Intn(len(items))]
}

// Shuffle 随机打乱切片（原地修改）。
func Shuffle[T any](items []T) {
	rng.Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})
}
