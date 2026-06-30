// Package grpc 提供压测脚本使用的 gRPC 客户端。
// 自动调用 ctx.Recorder 上报每次调用的耗时和结果，脚本无需手动记录指标。
//
// 典型用法：
//
//	// Setup 中建立连接（所有 VU 共享）
//	conn, err := grpc.Dial(ctx, ctx.Vars.Env("GRPC_ADDR"))
//
//	// Default 中发起调用
//	resp, err := grpc.Invoke(ctx, conn, "/order.OrderService/Create", reqBody)
package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/A0dongq1N/jarvan4-platform/spec"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Conn gRPC 连接包装，在 Setup 中创建，所有 VU 共享。
type Conn struct {
	cc *grpc.ClientConn
}

// Close 关闭连接，在 Teardown 中调用。
func (c *Conn) Close() error {
	return c.cc.Close()
}

// Response gRPC 响应。
type Response struct {
	// Status gRPC 状态码。
	Status codes.Code
	// Body proto 序列化后的字节，脚本使用 proto.Unmarshal 解析；
	// 若服务端返回 JSON（grpc-gateway 等），也可用 json.Unmarshal。
	Body []byte
}

// DialOption Dial 选项函数。
type DialOption func(*dialConfig)

type dialConfig struct {
	insecure bool
	timeout  time.Duration
}

// WithInsecure 不使用 TLS（开发/内网环境）。
func WithInsecure() DialOption {
	return func(c *dialConfig) { c.insecure = true }
}

// WithDialTimeout 连接超时。
func WithDialTimeout(d time.Duration) DialOption {
	return func(c *dialConfig) { c.timeout = d }
}

// CallOption 调用选项函数。
type CallOption func(*callConfig)

type callConfig struct {
	timeout  time.Duration
	metadata map[string]string
}

// WithCallTimeout 单次调用超时。
func WithCallTimeout(d time.Duration) CallOption {
	return func(c *callConfig) { c.timeout = d }
}

// WithMetadata 附加 gRPC metadata（等价于 HTTP header）。
func WithMetadata(key, value string) CallOption {
	return func(c *callConfig) {
		if c.metadata == nil {
			c.metadata = make(map[string]string)
		}
		c.metadata[key] = value
	}
}

// Dial 建立 gRPC 连接。
// target 格式：host:port，如 "localhost:9090"。
// 默认不使用 TLS，生产环境建议去掉 WithInsecure()。
func Dial(ctx *spec.RunContext, target string, opts ...DialOption) (*Conn, error) {
	cfg := &dialConfig{insecure: true, timeout: 10 * time.Second}
	for _, o := range opts {
		o(cfg)
	}

	var dialOpts []grpc.DialOption
	if cfg.insecure {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	dialCtx := context.Background()
	if cfg.timeout > 0 {
		var cancel context.CancelFunc
		dialCtx, cancel = context.WithTimeout(dialCtx, cfg.timeout)
		defer cancel()
	}

	cc, err := grpc.DialContext(dialCtx, target, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", target, err)
	}
	return &Conn{cc: cc}, nil
}

// Invoke 发起 unary gRPC 调用，内部自动调用 ctx.Recorder 上报指标。
//
// method 格式："/package.Service/Method"，如 "/order.OrderService/CreateOrder"。
// req 支持：proto.Message、map[string]interface{}（自动 JSON 序列化）、[]byte（直接发送）。
func Invoke(ctx *spec.RunContext, conn *Conn, method string, req interface{}, opts ...CallOption) (*Response, error) {
	start := time.Now()
	resp, err := invoke(ctx, conn, method, req, opts...)
	duration := time.Since(start)

	if ctx != nil && ctx.Recorder != nil {
		ctx.Recorder.Record(method, duration, err)
	}

	return resp, err
}

func invoke(ctx *spec.RunContext, conn *Conn, method string, req interface{}, opts ...CallOption) (*Response, error) {
	cfg := &callConfig{}
	for _, o := range opts {
		o(cfg)
	}

	callCtx := context.Background()
	if cfg.timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(callCtx, cfg.timeout)
		defer cancel()
	}
	if len(cfg.metadata) > 0 {
		md := metadata.New(cfg.metadata)
		callCtx = metadata.NewOutgoingContext(callCtx, md)
	}

	// 序列化请求体
	var reqBytes []byte
	switch v := req.(type) {
	case []byte:
		reqBytes = v
	case proto.Message:
		var err error
		reqBytes, err = proto.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal proto: %w", err)
		}
	default:
		var err error
		reqBytes, err = json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal json: %w", err)
		}
	}

	// 使用原始字节调用（codec: proto）
	var respBytes []byte
	err := conn.cc.Invoke(callCtx, method, reqBytes, &respBytes)
	if err != nil {
		st, _ := status.FromError(err)
		return &Response{Status: st.Code()}, fmt.Errorf("grpc invoke: %w", err)
	}

	return &Response{Status: codes.OK, Body: respBytes}, nil
}
