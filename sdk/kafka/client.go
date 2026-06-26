// Package kafka 提供压测脚本使用的 Kafka 生产者客户端。
// 自动调用 ctx.Recorder 上报每次 Produce 的耗时和结果，label 为 "kafka.Produce"。
//
// 典型用法：
//
//	// Setup 中创建 Producer（所有 VU 共享）
//	p, err := kafka.NewProducer(ctx, []string{ctx.Vars.Env("KAFKA_BROKERS")})
//
//	// Default 中发送消息
//	err = kafka.Produce(ctx, p, "order-topic", msgBytes)
package kafka

import (
	"fmt"
	"time"

	"github.com/Aodongq1n/jarvan4-platform/spec"
	"github.com/IBM/sarama"
)

// Producer Kafka 生产者，在 Setup 中创建，所有 VU 共享。
type Producer struct {
	p sarama.SyncProducer
}

// ProducerOption 生产者选项函数。
type ProducerOption func(*sarama.Config)

// WithRequiredAcks 设置 ack 级别（默认 WaitForAll）。
func WithRequiredAcks(acks sarama.RequiredAcks) ProducerOption {
	return func(c *sarama.Config) { c.Producer.RequiredAcks = acks }
}

// WithCompression 设置压缩算法（默认不压缩）。
func WithCompression(codec sarama.CompressionCodec) ProducerOption {
	return func(c *sarama.Config) { c.Producer.Compression = codec }
}

// MessageOption 消息选项函数。
type MessageOption func(*sarama.ProducerMessage)

// WithKey 指定消息 key（影响 partition 路由）。
func WithKey(key string) MessageOption {
	return func(m *sarama.ProducerMessage) { m.Key = sarama.StringEncoder(key) }
}

// WithPartition 指定目标 partition（-1 表示自动选择）。
func WithPartition(partition int32) MessageOption {
	return func(m *sarama.ProducerMessage) { m.Partition = partition }
}

// NewProducer 创建同步 Kafka 生产者。
// brokers 格式：[]string{"host1:9092", "host2:9092"}。
func NewProducer(ctx *spec.RunContext, brokers []string, opts ...ProducerOption) (*Producer, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.Return.Errors = true
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Net.DialTimeout = 10 * time.Second
	cfg.Net.ReadTimeout = 10 * time.Second
	cfg.Net.WriteTimeout = 10 * time.Second

	for _, o := range opts {
		o(cfg)
	}

	p, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka new producer %v: %w", brokers, err)
	}
	return &Producer{p: p}, nil
}

// Close 关闭生产者，在 Teardown 中调用。
func (p *Producer) Close() error {
	return p.p.Close()
}

// Produce 发送消息到指定 topic，自动上报 "kafka.Produce" 指标。
func Produce(ctx *spec.RunContext, p *Producer, topic string, msg []byte, opts ...MessageOption) error {
	pm := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(msg),
	}
	for _, o := range opts {
		o(pm)
	}

	start := time.Now()
	_, _, err := p.p.SendMessage(pm)
	duration := time.Since(start)

	if ctx != nil && ctx.Recorder != nil {
		ctx.Recorder.Record("kafka.Produce", duration, err)
	}
	return err
}
