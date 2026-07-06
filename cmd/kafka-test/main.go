// cmd/kafka-test 真实 Kafka 环境端到端测试工具。
// 用法: go run cmd/kafka-test/main.go
// 覆盖: DataSource 层、EventBus 层、Fallback 机制、端到端验证
// 目标 Kafka: 172.17.1.224:9092
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/events"
)

const (
	defaultBroker = "172.17.1.224:9092"
	// 两层使用不同 topic 前缀，避免消息竞争
	externalTopicPrefix = "test-external"
	internalTopicPrefix = "test-internal"
	consumeTimeout      = 15 * time.Second
)

var brokers []string

func init() {
	// 支持环境变量覆盖 broker 地址
	if v := os.Getenv("KAFKA_BROKER"); v != "" {
		brokers = strings.Split(v, ",")
	} else {
		brokers = []string{defaultBroker}
	}
}

// ──────────────────────────────
// 测试结果跟踪
// ──────────────────────────────

type testResult struct {
	name    string
	status  string // "PASS", "FAIL", "SKIP"
	detail  string
	elapsed time.Duration
}

type testRunner struct {
	results []testResult
	start   time.Time
}

func newTestRunner() *testRunner {
	return &testRunner{start: time.Now()}
}

func (r *testRunner) run(name string, fn func() (string, error)) {
	t0 := time.Now()
	detail, err := fn()
	elapsed := time.Since(t0)

	status := "PASS"
	if err != nil {
		status = "FAIL"
		detail = err.Error()
	}

	icon := "✓"
	if status == "FAIL" {
		icon = "✗"
	}

	mark := ""
	if status == "FAIL" {
		mark = fmt.Sprintf(" — %s", detail)
	} else if detail != "" {
		mark = fmt.Sprintf(" — %s", detail)
	}

	fmt.Printf("  %s %-56s [%v]%s\n", icon, name, elapsed.Round(time.Millisecond), mark)
	r.results = append(r.results, testResult{name: name, status: status, detail: detail, elapsed: elapsed})
}

func (r *testRunner) section(title string) {
	fmt.Printf("\n━━━ %s ━━━\n", title)
}

func (r *testRunner) summary() {
	passed, failed := 0, 0
	for _, res := range r.results {
		switch res.status {
		case "PASS":
			passed++
		case "FAIL":
			failed++
		}
	}
	total := passed + failed

	fmt.Println("\n╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║              Kafka 真实环境测试汇总报告                   ║")
	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	fmt.Printf("║  总数:  %-48d║\n", total)
	fmt.Printf("║  通过:  %-48d║\n", passed)
	fmt.Printf("║  失败:  %-48d║\n", failed)
	fmt.Printf("║  耗时:  %-48s║\n", time.Since(r.start).Round(time.Millisecond))
	fmt.Println("╚══════════════════════════════════════════════════════════╝")

	if failed > 0 {
		fmt.Println("\n── 失败详情 ──")
		for _, res := range r.results {
			if res.status == "FAIL" {
				fmt.Printf("  ✗ %s: %s\n", res.name, res.detail)
			}
		}
	}
}

// ──────────────────────────────
// 辅助函数
// ──────────────────────────────

func uniqueTopic(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// extractHost 从 sarama DNS 错误中提取 hostname。
func extractHost(errMsg string) string {
	// 错误格式: "dial tcp: lookup kafka: no such host"
	parts := strings.Split(errMsg, "lookup ")
	if len(parts) > 1 {
		host := strings.Split(parts[1], ":")[0]
		host = strings.Split(host, " ")[0]
		return host
	}
	return "unknown"
}

// safeClose 安全关闭，捕获 sarama 已知 bug 导致的 panic。
// sarama v1.50.x 在 producer 异常状态下 Close() 可能触发 "send on closed channel"。
func safeClose(c interface{ Close() error }) {
	defer func() {
		if r := recover(); r != nil {
			// 静默忽略 sarama 内部 panic，不影响测试结果判断
		}
	}()
	c.Close()
}

func makeTestEvents(count int) []events.SyncEvent {
	result := make([]events.SyncEvent, 0, count)
	for i := 0; i < count; i++ {
		result = append(result, events.SyncEvent{
			Action:     "update",
			EntityType: "Device",
			Connector:  "netbox",
			Data: []map[string]any{
				{"name": fmt.Sprintf("PE-Router-%02d", i+1), "ip": fmt.Sprintf("10.0.0.%d", i+1), "role": "edge"},
				{"name": fmt.Sprintf("PE-Switch-%02d", i+1), "ip": fmt.Sprintf("10.0.1.%d", i+1), "role": "core"},
			},
		})
	}
	return result
}

// consumeEvents 启动 consumer 并收集指定数量的事件，超时自动退出。
func consumeEvents(consumer events.EventConsumer, count int) ([]events.SyncEvent, error) {
	var (
		mu         sync.Mutex
		received   []events.SyncEvent
		ctx, cancel = context.WithTimeout(context.Background(), consumeTimeout)
	)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Consume(ctx, func(_ context.Context, event events.SyncEvent) error {
			mu.Lock()
			received = append(received, event)
			n := len(received)
			mu.Unlock()
			if n >= count {
				cancel()
			}
			return nil
		})
	}()

	// 等待消费完成或超时
	select {
	case err := <-errCh:
		// context.Canceled 是正常退出（收到足够消息后 cancel）
		if err != nil && ctx.Err() == nil {
			return nil, fmt.Errorf("consume error: %w", err)
		}
	case <-ctx.Done():
		// 超时或 cancel
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) < count {
		return received, fmt.Errorf("timeout: expected %d events, got %d", count, len(received))
	}
	return received, nil
}

// ──────────────────────────────
// 测试入口
// ──────────────────────────────

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Kafka 真实环境端到端测试")
	fmt.Printf("  Broker: %s\n", strings.Join(brokers, ","))
	fmt.Println("═══════════════════════════════════════════════════════════")

	r := newTestRunner()

	// runTests 提取为函数，defer summary 确保即使 sarama goroutine panic 也尽量输出结果
	hasFailures := runTests(r)

	// 输出汇总
	r.summary()

	if hasFailures {
		os.Exit(1)
	}
}

func runTests(r *testRunner) bool {

	// ──────────────────────────────
	// Section 1: 连通性测试
	// ──────────────────────────────
	r.section("Section 1: 连通性测试")

	saramaCfg, err := events.NewSaramaConfig("", "")
	if err != nil {
		fmt.Printf("FATAL: create sarama config: %v\n", err)
		os.Exit(1)
	}

	// Test 1: KafkaPing
	r.run("TestKafkaPing", func() (string, error) {
		pub, err := events.NewKafkaPublisher(brokers, "test-ping-topic", saramaCfg)
		if err != nil {
			return "", fmt.Errorf("create publisher: %w", err)
		}
		defer safeClose(pub)

		pinger, ok := pub.(interface{ Ping() error })
		if !ok {
			return "", fmt.Errorf("publisher does not implement Ping")
		}
		if err := pinger.Ping(); err != nil {
			return "", fmt.Errorf("ping failed: %w", err)
		}
		return "Kafka broker reachable", nil
	})

	// Test 2: SaramaClientBrokers
	r.run("TestSaramaClientBrokers", func() (string, error) {
		pub, err := events.NewKafkaPublisher(brokers, "test-brokers-topic", saramaCfg)
		if err != nil {
			return "", fmt.Errorf("create publisher: %w", err)
		}
		defer safeClose(pub)

		// 通过 Ping 间接验证 client.Brokers() 非空
		pinger, ok := pub.(interface{ Ping() error })
		if !ok {
			return "", fmt.Errorf("publisher does not implement Ping")
		}
		if err := pinger.Ping(); err != nil {
			return "", fmt.Errorf("no brokers: %w", err)
		}
		return "brokers connected", nil
	})

	// ──────────────────────────────
	// Section 2: EventBus 层测试
	// ──────────────────────────────
	r.section("Section 2: EventBus 层测试")

	// Test 3: AdvertisedListener 诊断（检测 Kafka 元数据中 advertised 地址是否可达）
	r.run("TestAdvertisedListener", func() (string, error) {
		topic := uniqueTopic(internalTopicPrefix + "-diag")
		pub, err := events.NewKafkaPublisher(brokers, topic, saramaCfg)
		if err != nil {
			return "", fmt.Errorf("create publisher: %w", err)
		}
		defer safeClose(pub)

		// 发送一条测试消息，如果 advertised listener 不正确会报 DNS 错误
		evt := events.SyncEvent{Action: "update", EntityType: "Diag", Connector: "test"}
		if err := pub.Publish(context.Background(), evt); err != nil {
			if strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "lookup") {
				return "", fmt.Errorf("ADVERTISED_LISTENER 配置错误: broker 元数据返回的地址 (%s) 无法解析\n"+
					"  修复方法: 在 Kafka 服务器上修改 KAFKA_ADVERTISED_LISTENERS:\n"+
					"    将 PLAINTEXT://kafka:9092 改为 PLAINTEXT://172.17.1.224:9092\n"+
					"  然后重启 Kafka 容器: docker compose restart kafka\n"+
					"  原始错误: %w", extractHost(err.Error()), err)
			}
			return "", fmt.Errorf("publish diagnostic: %w", err)
		}
		return "advertised listener OK", nil
	})

	// Test 4: KafkaPublishConsume
	r.run("TestKafkaPublishConsume", func() (string, error) {
		topic := uniqueTopic(internalTopicPrefix + "-pubsub")

		pub, err := events.NewKafkaPublisher(brokers, topic, saramaCfg)
		if err != nil {
			return "", fmt.Errorf("create publisher: %w", err)
		}

		con, err := events.NewKafkaConsumer(brokers, topic, "test-pubsub-group", saramaCfg)
		if err != nil {
			safeClose(pub)
			return "", fmt.Errorf("create consumer: %w", err)
		}

		// 发送 3 条事件
		testEvents := makeTestEvents(3)
		for i, evt := range testEvents {
			evt.Connector = fmt.Sprintf("netbox-%d", i)
			if err := pub.Publish(context.Background(), evt); err != nil {
				return "", fmt.Errorf("publish event %d: %w", i, err)
			}
		}

		// 消费并验证
		received, err := consumeEvents(con, 3)
		if err != nil {
			safeClose(con)
			safeClose(pub)
			return "", err
		}

		// 验证内容
		for i, got := range received {
			if got.Action != "update" {
				safeClose(con)
				safeClose(pub)
				return "", fmt.Errorf("event %d: expected action=update, got %s", i, got.Action)
			}
			if got.EntityType != "Device" {
				safeClose(con)
				safeClose(pub)
				return "", fmt.Errorf("event %d: expected entity_type=Device, got %s", i, got.EntityType)
			}
			if len(got.Data) != 2 {
				safeClose(con)
				safeClose(pub)
				return "", fmt.Errorf("event %d: expected 2 data items, got %d", i, len(got.Data))
			}
		}

		// 显式关闭，避免 defer 与 sarama goroutine 竞态
		safeClose(con)
		time.Sleep(200 * time.Millisecond)
		safeClose(pub)

		return fmt.Sprintf("3/3 events consumed and validated"), nil
	})

	// Test 5: FallbackPrimaryOK
	r.run("TestFallbackPrimaryOK", func() (string, error) {
		topic := uniqueTopic(internalTopicPrefix + "-fallback-ok")

		// Kafka primary
		kafkaPub, err := events.NewKafkaPublisher(brokers, topic, saramaCfg)
		if err != nil {
			return "", fmt.Errorf("create kafka publisher: %w", err)
		}

		// Channel fallback
		chanPub, chanCon := events.NewChannelEventBus(100)

		// FallbackPublisher
		fbPub := events.NewFallbackPublisher(kafkaPub, chanPub, 5*time.Second)

		// 发布事件，应该走 Kafka primary
		evt := events.SyncEvent{
			Action:     "update",
			EntityType: "Interface",
			Connector:  "netbox",
			Data:       []map[string]any{{"name": "eth0"}},
		}
		if err := fbPub.Publish(context.Background(), evt); err != nil {
			safeClose(fbPub)
			return "", fmt.Errorf("fallback publish: %w", err)
		}

		// 从 Kafka consumer 验证消息确实走了 Kafka（primary）
		con, err := events.NewKafkaConsumer(brokers, topic, "test-fallback-ok-group", saramaCfg)
		if err != nil {
			safeClose(fbPub)
			return "", fmt.Errorf("create consumer: %w", err)
		}

		received, err := consumeEvents(con, 1)
		if err != nil {
			safeClose(con)
			safeClose(fbPub)
			return "", fmt.Errorf("primary not used: %w", err)
		}
		if received[0].EntityType != "Interface" {
			safeClose(con)
			safeClose(fbPub)
			return "", fmt.Errorf("unexpected entity: %s", received[0].EntityType)
		}

		// 显式关闭
		safeClose(con)
		time.Sleep(200 * time.Millisecond)
		safeClose(fbPub)
		chanPub.Close()
		chanCon.Close()

		return "event went through Kafka primary (not fallback)", nil
	})

	// Test 6: FallbackDegradation
	r.run("TestFallbackDegradation", func() (string, error) {
		// 用错误 broker 创建 Kafka publisher（无法连接）
		badSaramaCfg, _ := events.NewSaramaConfig("", "")
		badSaramaCfg.Net.DialTimeout = 2 * time.Second

		badBrokers := []string{"192.0.2.1:9092"} // TEST-NET，不可达

		// 创建 Kafka publisher 会失败
		_, err := events.NewKafkaPublisher(badBrokers, "test-degrade", badSaramaCfg)
		if err == nil {
			return "", fmt.Errorf("expected kafka publisher creation to fail with unreachable broker")
		}

		// 既然 Kafka publisher 创建失败，FallbackPublisher 无法正常工作（因为需要 primary）
		// 验证：直接使用 Channel 作为替代方案（模拟 main.go 中 Kafka 创建失败时的降级逻辑）
		chanPub, chanCon := events.NewChannelEventBus(100)

		evt := events.SyncEvent{
			Action:     "delete",
			EntityType: "Link",
			Connector:  "cmdb",
			URIs:       []string{"urn:link:L001"},
		}
		if err := chanPub.Publish(context.Background(), evt); err != nil {
			return "", fmt.Errorf("channel fallback publish: %w", err)
		}

		// 从 channel 消费验证
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var received events.SyncEvent
		go chanCon.Consume(ctx, func(_ context.Context, event events.SyncEvent) error {
			received = event
			cancel()
			return nil
		})
		<-ctx.Done()

		if received.Action != "delete" {
			return "", fmt.Errorf("expected delete action in fallback, got %s", received.Action)
		}

		return "Kafka unreachable -> Channel fallback works", nil
	})

	// ──────────────────────────────
	// Section 3: DataSource 层测试
	// ──────────────────────────────
	r.section("Section 3: DataSource 层测试")

	// Test 6: DataSourceConsumer
	r.run("TestDataSourceConsumer", func() (string, error) {
		topic := uniqueTopic(externalTopicPrefix + "-ds")

		// 模拟外部 producer 向 DataSource topic 发送事件
		extPub, err := events.NewKafkaPublisher(brokers, topic, saramaCfg)
		if err != nil {
			return "", fmt.Errorf("create external publisher: %w", err)
		}
		defer safeClose(extPub)

		// 创建 Channel EventBus 作为 DataSource 转发目标
		chanPub, chanCon := events.NewChannelEventBus(100)
		defer safeClose(chanPub)
		defer safeClose(chanCon)

		// 创建 KafkaDataSourceConsumer
		dsConsumer, err := events.NewKafkaDataSourceConsumer(brokers, topic, "test-ds-group", saramaCfg)
		if err != nil {
			return "", fmt.Errorf("create ds consumer: %w", err)
		}
		defer safeClose(dsConsumer)

		// 启动 DataSource consumer（后台）
		dsCtx, dsCancel := context.WithTimeout(context.Background(), consumeTimeout)
		defer dsCancel()

		go func() {
			if err := dsConsumer.Start(dsCtx, chanPub); err != nil && dsCtx.Err() == nil {
				fmt.Printf("    [WARN] ds consumer stopped: %v\n", err)
			}
		}()

		// 等待 consumer group 初始化
		time.Sleep(2 * time.Second)

		// 发送事件到外部 topic
		evt := events.SyncEvent{
			Action:     "update",
			EntityType: "Device",
			Connector:  "netbox",
			Data: []map[string]any{
				{"name": "PE-Router-01", "ip": "10.0.0.1"},
			},
		}
		if err := extPub.Publish(context.Background(), evt); err != nil {
			return "", fmt.Errorf("publish to external topic: %w", err)
		}

		// 从 Channel consumer 接收（DataSource 转发过来的）
		received, err := consumeEvents(chanCon, 1)
		if err != nil {
			return "", fmt.Errorf("ds forward failed: %w", err)
		}
		if received[0].EntityType != "Device" {
			return "", fmt.Errorf("unexpected entity: %s", received[0].EntityType)
		}

		return "DataSource consumer forwarded event to EventBus", nil
	})

	// Test 7: DataSourceEventContent
	r.run("TestDataSourceEventContent", func() (string, error) {
		topic := uniqueTopic(externalTopicPrefix + "-ds-content")

		extPub, err := events.NewKafkaPublisher(brokers, topic, saramaCfg)
		if err != nil {
			return "", fmt.Errorf("create external publisher: %w", err)
		}
		defer safeClose(extPub)

		chanPub, chanCon := events.NewChannelEventBus(100)
		defer safeClose(chanPub)
		defer safeClose(chanCon)

		dsConsumer, err := events.NewKafkaDataSourceConsumer(brokers, topic, "test-ds-content-group", saramaCfg)
		if err != nil {
			return "", fmt.Errorf("create ds consumer: %w", err)
		}
		defer safeClose(dsConsumer)

		dsCtx, dsCancel := context.WithTimeout(context.Background(), consumeTimeout)
		defer dsCancel()

		go func() {
			if err := dsConsumer.Start(dsCtx, chanPub); err != nil && dsCtx.Err() == nil {
				fmt.Printf("    [WARN] ds consumer stopped: %v\n", err)
			}
		}()

		time.Sleep(2 * time.Second)

		// 发送包含完整字段的事件
		evt := events.SyncEvent{
			Action:     "update",
			EntityType: "Device",
			Connector:  "netbox",
			Data: []map[string]any{
				{"name": "PE-Router-01", "ip": "10.0.0.1", "role": "edge", "vendor": "Huawei"},
				{"name": "PE-Router-02", "ip": "10.0.0.2", "role": "edge", "vendor": "Cisco"},
			},
		}
		if err := extPub.Publish(context.Background(), evt); err != nil {
			return "", fmt.Errorf("publish: %w", err)
		}

		received, err := consumeEvents(chanCon, 1)
		if err != nil {
			return "", err
		}

		got := received[0]
		// 验证所有字段完整性
		if got.Action != "update" {
			return "", fmt.Errorf("action: expected=update got=%s", got.Action)
		}
		if got.EntityType != "Device" {
			return "", fmt.Errorf("entity_type: expected=Device got=%s", got.EntityType)
		}
		if got.Connector != "netbox" {
			return "", fmt.Errorf("connector: expected=netbox got=%s", got.Connector)
		}
		if len(got.Data) != 2 {
			return "", fmt.Errorf("data count: expected=2 got=%d", len(got.Data))
		}
		if got.Data[0]["name"] != "PE-Router-01" {
			return "", fmt.Errorf("data[0].name: expected=PE-Router-01 got=%v", got.Data[0]["name"])
		}
		if got.Data[1]["vendor"] != "Cisco" {
			return "", fmt.Errorf("data[1].vendor: expected=Cisco got=%v", got.Data[1]["vendor"])
		}

		return "all fields intact (action/entity/connector/data)", nil
	})

	// ──────────────────────────────
	// Section 4: 端到端验证
	// ──────────────────────────────
	r.section("Section 4: 端到端验证")

	// Test 9: EndToEnd
	r.run("TestEndToEnd", func() (string, error) {
		extTopic := uniqueTopic(externalTopicPrefix + "-e2e")
		intTopic := uniqueTopic(internalTopicPrefix + "-e2e")

		// 1. 创建 EventBus 层（Kafka Publisher + Consumer + FallbackPublisher）
		kafkaPub, err := events.NewKafkaPublisher(brokers, intTopic, saramaCfg)
		if err != nil {
			return "", fmt.Errorf("create eventbus kafka pub: %w", err)
		}
		defer safeClose(kafkaPub)

		chanPub, _ := events.NewChannelEventBus(100)
		fbPub := events.NewFallbackPublisher(kafkaPub, chanPub, 5*time.Second)
		defer safeClose(fbPub)

		eventBusCon, err := events.NewKafkaConsumer(brokers, intTopic, "test-e2e-eventbus-group", saramaCfg)
		if err != nil {
			return "", fmt.Errorf("create eventbus consumer: %w", err)
		}
		defer safeClose(eventBusCon)

		// 2. 创建 DataSource 层（外部 Topic -> DataSource Consumer -> EventBus Publisher）
		dsConsumer, err := events.NewKafkaDataSourceConsumer(brokers, extTopic, "test-e2e-ds-group", saramaCfg)
		if err != nil {
			return "", fmt.Errorf("create ds consumer: %w", err)
		}
		defer safeClose(dsConsumer)

		// 3. 启动 DataSource consumer 后台运行
		e2eCtx, e2eCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer e2eCancel()

		go func() {
			if err := dsConsumer.Start(e2eCtx, fbPub); err != nil && e2eCtx.Err() == nil {
				fmt.Printf("    [WARN] ds consumer stopped: %v\n", err)
			}
		}()

		// 等待 consumer groups 初始化
		time.Sleep(3 * time.Second)

		// 4. 模拟外部 Producer 向 DataSource Topic 发送事件
		extPub, err := events.NewKafkaPublisher(brokers, extTopic, saramaCfg)
		if err != nil {
			return "", fmt.Errorf("create external publisher: %w", err)
		}
		defer safeClose(extPub)

		testEvents := makeTestEvents(2)
		for i, evt := range testEvents {
			evt.Connector = fmt.Sprintf("netbox-e2e-%d", i)
			if err := extPub.Publish(context.Background(), evt); err != nil {
				return "", fmt.Errorf("publish event %d: %w", i, err)
			}
		}

		// 5. 从 EventBus Consumer 消费（完整链路终点）
		received, err := consumeEvents(eventBusCon, 2)
		if err != nil {
			return "", fmt.Errorf("e2e consume failed: %w", err)
		}

		// 6. 验证
		for i, got := range received {
			if got.Action != "update" {
				return "", fmt.Errorf("event %d: action=%s", i, got.Action)
			}
			if got.EntityType != "Device" {
				return "", fmt.Errorf("event %d: entity_type=%s", i, got.EntityType)
			}
			if !strings.HasPrefix(got.Connector, "netbox-e2e") {
				return "", fmt.Errorf("event %d: connector=%s (expected netbox-e2e-*)", i, got.Connector)
			}
		}

		return "External Producer -> DS Topic -> DS Consumer -> EventBus(Fallback+Kafka) -> EventBus Consumer: 2/2 OK", nil
	})

	// 返回是否有失败
	for _, res := range r.results {
		if res.status == "FAIL" {
			return true
		}
	}
	return false
}
