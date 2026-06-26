// Package log 提供 spec.Logger 接口的标准实现，由 Worker 注入到 RunContext。
// 日志通过 channel 批量收集后上报到 Master 日志流。
package log

import (
	"fmt"
	"sync"
	"time"
)

// Entry 日志条目。
type Entry struct {
	Level     string
	Message   string
	Timestamp time.Time
	VUId      int
	WorkerID  string
}

// Logger 实现 spec.Logger 接口，日志写入内部 channel 由 Worker 异步上报。
type Logger struct {
	vuID     int
	workerID string
	ch       chan<- Entry
	mu       sync.Mutex
}

// New 创建 Logger（每个 VU 独立实例，共享同一个 channel）。
func New(vuID int, workerID string, ch chan<- Entry) *Logger {
	return &Logger{vuID: vuID, workerID: workerID, ch: ch}
}

func (l *Logger) Debug(format string, args ...interface{}) {
	l.emit("debug", format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.emit("info", format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.emit("warn", format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.emit("error", format, args...)
}

func (l *Logger) emit(level, format string, args ...interface{}) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	entry := Entry{
		Level:     level,
		Message:   msg,
		Timestamp: time.Now(),
		VUId:      l.vuID,
		WorkerID:  l.workerID,
	}
	// 非阻塞写入，channel 满时丢弃（防止日志阻塞压测引擎）
	select {
	case l.ch <- entry:
	default:
	}
}
