// Package tcp 提供压测脚本使用的原生 TCP 客户端。
// 适用于自定义二进制协议场景。Send/Recv 自动上报指标。
package tcp

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/Aodongq1n/jarvan4-platform/spec"
)

// Conn TCP 连接。
type Conn struct {
	conn net.Conn
}

// ConnOption 连接选项函数。
type ConnOption func(*connConfig)

type connConfig struct {
	timeout time.Duration
}

// WithTimeout 设置连接超时。
func WithTimeout(d time.Duration) ConnOption {
	return func(c *connConfig) { c.timeout = d }
}

// Connect 建立 TCP 连接。addr 格式：host:port。
func Connect(ctx *spec.RunContext, addr string, opts ...ConnOption) (*Conn, error) {
	cfg := &connConfig{timeout: 10 * time.Second}
	for _, o := range opts {
		o(cfg)
	}

	conn, err := net.DialTimeout("tcp", addr, cfg.timeout)
	if err != nil {
		return nil, fmt.Errorf("tcp connect %s: %w", addr, err)
	}
	return &Conn{conn: conn}, nil
}

// Close 关闭连接。
func (c *Conn) Close() error {
	return c.conn.Close()
}

// SetDeadline 设置读写超时截止时间。
func (c *Conn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

// Send 发送字节数据，自动上报 "tcp.Send" 指标。
func Send(ctx *spec.RunContext, conn *Conn, data []byte) error {
	start := time.Now()
	_, err := conn.conn.Write(data)
	duration := time.Since(start)

	if ctx != nil && ctx.Recorder != nil {
		ctx.Recorder.Record("tcp.Send", duration, err)
	}
	return err
}

// Recv 读取指定字节数，自动上报 "tcp.Recv" 指标。
func Recv(ctx *spec.RunContext, conn *Conn, n int) ([]byte, error) {
	buf := make([]byte, n)
	start := time.Now()
	_, err := io.ReadFull(conn.conn, buf)
	duration := time.Since(start)

	if ctx != nil && ctx.Recorder != nil {
		ctx.Recorder.Record("tcp.Recv", duration, err)
	}
	if err != nil {
		return nil, err
	}
	return buf, nil
}
