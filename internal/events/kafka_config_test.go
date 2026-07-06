// Package events Kafka 配置工厂单元测试
package events

import (
	"testing"
	"time"

	"github.com/IBM/sarama"
)

// TestNewSaramaConfig_NoSASL 验证无 SASL 时默认配置正确。
func TestNewSaramaConfig_NoSASL(t *testing.T) {
	cfg, err := NewSaramaConfig("", "")
	if err != nil {
		t.Fatalf("NewSaramaConfig() error = %v", err)
	}

	// Producer 配置
	if cfg.Producer.RequiredAcks != sarama.WaitForAll {
		t.Errorf("RequiredAcks = %v, want WaitForAll", cfg.Producer.RequiredAcks)
	}
	if cfg.Producer.Retry.Max != 3 {
		t.Errorf("Retry.Max = %d, want 3", cfg.Producer.Retry.Max)
	}
	if !cfg.Producer.Return.Successes {
		t.Error("Return.Successes = false, want true")
	}

	// Consumer 配置
	if !cfg.Consumer.Return.Errors {
		t.Error("Consumer.Return.Errors = false, want true")
	}
	if cfg.Consumer.Offsets.Initial != sarama.OffsetOldest {
		t.Errorf("Offsets.Initial = %v, want OffsetOldest", cfg.Consumer.Offsets.Initial)
	}

	// SASL 应禁用
	if cfg.Net.SASL.Enable {
		t.Error("SASL.Enable = true, want false when no credentials")
	}

	// 超时
	if cfg.Net.DialTimeout != 10*time.Second {
		t.Errorf("DialTimeout = %v, want 10s", cfg.Net.DialTimeout)
	}
}

// TestNewSaramaConfig_WithSASL 验证 SASL 配置正确。
func TestNewSaramaConfig_WithSASL(t *testing.T) {
	cfg, err := NewSaramaConfig("admin", "secret")
	if err != nil {
		t.Fatalf("NewSaramaConfig() error = %v", err)
	}

	if !cfg.Net.SASL.Enable {
		t.Fatal("SASL.Enable = false, want true when credentials provided")
	}
	if cfg.Net.SASL.User != "admin" {
		t.Errorf("SASL.User = %q, want %q", cfg.Net.SASL.User, "admin")
	}
	if cfg.Net.SASL.Password != "secret" {
		t.Errorf("SASL.Password = %q, want %q", cfg.Net.SASL.Password, "secret")
	}
	if cfg.Net.SASL.Mechanism != sarama.SASLTypePlaintext {
		t.Errorf("SASL.Mechanism = %v, want PLAIN", cfg.Net.SASL.Mechanism)
	}
}
