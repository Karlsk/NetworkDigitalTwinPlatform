// Package events 提供带 Fallback 的 EventPublisher 实现。
// 当 EventBus 层的 Kafka 不可用时，自动降级到内存 Channel。
package events

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// pinger 窄接口，用于探测 Publisher 连通性。
// kafkaPublisher 实现此接口；fallback tryRecover 通过 Ping 判断 primary 是否恢复。
type pinger interface {
	Ping() error
}

// fallbackPublisher 带 Fallback 的 EventPublisher（仅 EventBus 层使用）。
// 当 primary（Kafka）Publish 失败时自动降级到 fallback（Channel），
// 后台 goroutine 通过 Ping 探测 primary 连通性，恢复后自动切回。
type fallbackPublisher struct {
	primary       EventPublisher
	fallback      EventPublisher
	mu            sync.RWMutex // 保护 primaryOK
	primaryOK     bool
	retryInterval time.Duration
}

// NewFallbackPublisher 创建带 Fallback 的 Publisher（仅 EventBus 层使用）。
// primary 实现 pinger 接口时，tryRecover 会调用 Ping 探测连通性；
// 否则 tryRecover 在首次 ticker 后直接认为恢复。
func NewFallbackPublisher(primary, fallback EventPublisher, retryInterval time.Duration) EventPublisher {
	return &fallbackPublisher{
		primary:       primary,
		fallback:      fallback,
		primaryOK:     true,
		retryInterval: retryInterval,
	}
}

// Publish 发布事件。primary 正常时使用 primary，失败时降级到 fallback 并启动 tryRecover。
func (p *fallbackPublisher) Publish(ctx context.Context, event SyncEvent) error {
	p.mu.RLock()
	ok := p.primaryOK
	p.mu.RUnlock()

	if ok {
		if err := p.primary.Publish(ctx, event); err != nil {
			slog.Warn("kafka publish failed, falling back to channel",
				"error", err)
			p.mu.Lock()
			p.primaryOK = false
			p.mu.Unlock()
			go p.tryRecover(ctx)
			return p.fallback.Publish(ctx, event)
		}
		return nil
	}
	return p.fallback.Publish(ctx, event)
}

// tryRecover 后台定期探测 primary 连通性，成功则恢复 primaryOK。
func (p *fallbackPublisher) tryRecover(ctx context.Context) {
	ticker := time.NewTicker(p.retryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			slog.Info("attempting to recover kafka connection")
			if pg, ok := p.primary.(pinger); ok {
				if err := pg.Ping(); err != nil {
					slog.Warn("kafka ping failed, still using fallback",
						"error", err)
					continue
				}
				slog.Info("kafka connection recovered via ping")
			} else {
				// primary 不实现 pinger，保守恢复
				slog.Info("primary does not implement pinger, assuming recovered")
			}
			p.mu.Lock()
			p.primaryOK = true
			p.mu.Unlock()
			return
		}
	}
}

// Close 同时关闭 primary 和 fallback，收集所有 error。
func (p *fallbackPublisher) Close() error {
	var errs []error
	if err := p.primary.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close primary: %w", err))
	}
	if err := p.fallback.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close fallback: %w", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("close publishers: %v", errs)
	}
	return nil
}
