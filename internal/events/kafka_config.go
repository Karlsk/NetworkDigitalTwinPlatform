// Package events 提供 sarama Kafka 客户端配置工厂。
package events

import (
	"time"

	"github.com/IBM/sarama"
)

// NewSaramaConfig 创建 sarama 配置。
// saslUser 非空时启用 SASL/PLAINTEXT 认证。
func NewSaramaConfig(saslUser, saslPass string) (*sarama.Config, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.RequiredAcks = sarama.WaitForAll  // 等待所有副本确认
	cfg.Producer.Retry.Max = 3                      // 最多重试 3 次
	cfg.Producer.Return.Successes = true            // SyncProducer 必须设置
	cfg.Consumer.Return.Errors = true
	cfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest // 从最早消息开始消费

	// SASL 认证（可选）
	if saslUser != "" {
		cfg.Net.SASL.Enable = true
		cfg.Net.SASL.User = saslUser
		cfg.Net.SASL.Password = saslPass
		cfg.Net.SASL.Mechanism = sarama.SASLTypePlaintext
	}

	cfg.Net.DialTimeout = 10 * time.Second
	cfg.Net.ReadTimeout = 10 * time.Second
	cfg.Net.WriteTimeout = 10 * time.Second

	return cfg, nil
}
