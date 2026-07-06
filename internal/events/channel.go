// Package events 提供 Channel（内存）事件总线实现（V1 兼容）。
package events

import (
	"context"
	"errors"
	"log/slog"
)

// channelPublisher 基于内存 Channel 的 EventPublisher 实现（V1 兼容）。
type channelPublisher struct {
	ch chan SyncEvent
}

// channelConsumer 基于内存 Channel 的 EventConsumer 实现（V1 兼容）。
type channelConsumer struct {
	ch chan SyncEvent
}

// NewChannelEventBus 创建共享 channel 的 Publisher/Consumer 对。
// 两者共享同一个 chan SyncEvent，类似 Kafka 的 topic 概念。
// bufferSize 控制缓冲区容量，满时 Publish 返回 error。
func NewChannelEventBus(bufferSize int) (EventPublisher, EventConsumer) {
	ch := make(chan SyncEvent, bufferSize)
	return &channelPublisher{ch: ch}, &channelConsumer{ch: ch}
}

// Publish 非阻塞写入事件到 channel。
// channel 满时立即返回 error（调用方应返回 503）。
func (p *channelPublisher) Publish(_ context.Context, event SyncEvent) error {
	select {
	case p.ch <- event:
		return nil
	default:
		return errors.New("event queue full")
	}
}

// Close 关闭发布者（Channel 实现无需释放资源）。
func (p *channelPublisher) Close() error { return nil }

// Consume 启动消费循环，阻塞直到 ctx 取消。
// 每收到一个事件，调用 handler 处理。handler 失败时记录日志但不中断消费。
func (c *channelConsumer) Consume(ctx context.Context, handler func(ctx context.Context, event SyncEvent) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-c.ch:
			if err := handler(ctx, event); err != nil {
				slog.Error("event handler failed", "error", err)
			}
		}
	}
}

// Close 关闭消费者（Channel 实现无需释放资源）。
func (c *channelConsumer) Close() error { return nil }
