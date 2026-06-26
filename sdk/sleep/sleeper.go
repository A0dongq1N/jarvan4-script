// Package sleep 提供 spec.Sleeper 接口的标准实现，由 Worker 注入到 RunContext。
// 与 time.Sleep 的区别：可被 stopCh 中断，压测停止时 VU 不会卡在 sleep 上。
package sleep

import (
	"time"
)

// Sleeper 实现 spec.Sleeper 接口。
type Sleeper struct {
	stopCh <-chan struct{}
}

// New 创建 Sleeper，stopCh 关闭时所有 Sleep 立即返回。
func New(stopCh <-chan struct{}) *Sleeper {
	return &Sleeper{stopCh: stopCh}
}

// Sleep 暂停当前 VU，duration 期间若 stopCh 关闭会提前返回。
func (s *Sleeper) Sleep(duration time.Duration) {
	select {
	case <-time.After(duration):
	case <-s.stopCh:
	}
}
