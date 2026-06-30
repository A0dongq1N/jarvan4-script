// Package ws 提供压测脚本使用的 WebSocket 客户端。
// Send/Recv 自动调用 ctx.Recorder 上报指标，label 分别为 "ws.Send"、"ws.Recv"。
//
// 典型用法：
//
//	// Default 中建立连接（WebSocket 通常每个 VU 独立连接）
//	conn, err := ws.Connect(ctx, ctx.Vars.Env("WS_URL"))
//	defer conn.Close()
//	err = ws.Send(ctx, conn, []byte(`{"action":"ping"}`))
//	msg, err := ws.Recv(ctx, conn)
package ws

import (
	"fmt"
	"time"

	"github.com/A0dongq1N/jarvan4-platform/spec"
	"github.com/gorilla/websocket"
)

// Conn WebSocket 连接。通常每个 VU goroutine 独立持有一个连接。
type Conn struct {
	ws *websocket.Conn
}

// ConnOption 连接选项函数。
type ConnOption func(*websocket.Dialer)

// WithHandshakeTimeout 设置握手超时。
func WithHandshakeTimeout(d time.Duration) ConnOption {
	return func(dialer *websocket.Dialer) { dialer.HandshakeTimeout = d }
}

// Connect 建立 WebSocket 连接。
// url 格式：ws://host:port/path 或 wss://host:port/path。
func Connect(ctx *spec.RunContext, url string, opts ...ConnOption) (*Conn, error) {
	dialer := websocket.DefaultDialer
	for _, o := range opts {
		o(dialer)
	}

	ws, _, err := dialer.Dial(url, nil)
	if err != nil {
		return nil, fmt.Errorf("ws connect %s: %w", url, err)
	}
	return &Conn{ws: ws}, nil
}

// Close 关闭连接。
func (c *Conn) Close() error {
	return c.ws.Close()
}

// Send 发送消息，自动上报 "ws.Send" 指标。
func Send(ctx *spec.RunContext, conn *Conn, msg []byte) error {
	start := time.Now()
	err := conn.ws.WriteMessage(websocket.TextMessage, msg)
	duration := time.Since(start)

	if ctx != nil && ctx.Recorder != nil {
		ctx.Recorder.Record("ws.Send", duration, err)
	}
	return err
}

// Recv 接收消息，自动上报 "ws.Recv" 指标（耗时包含等待时间）。
func Recv(ctx *spec.RunContext, conn *Conn) ([]byte, error) {
	start := time.Now()
	_, msg, err := conn.ws.ReadMessage()
	duration := time.Since(start)

	if ctx != nil && ctx.Recorder != nil {
		ctx.Recorder.Record("ws.Recv", duration, err)
	}
	return msg, err
}

// SendBinary 发送二进制消息，自动上报 "ws.Send" 指标。
func SendBinary(ctx *spec.RunContext, conn *Conn, msg []byte) error {
	start := time.Now()
	err := conn.ws.WriteMessage(websocket.BinaryMessage, msg)
	duration := time.Since(start)

	if ctx != nil && ctx.Recorder != nil {
		ctx.Recorder.Record("ws.Send", duration, err)
	}
	return err
}
