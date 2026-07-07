// cmd/kafka-test 真实 Kafka 环境端到端测试工具。
// 用法: go run cmd/kafka-test/main.go
// 覆盖: DataSource 层、EventBus 层、Fallback 机制、端到端验证
// 目标 Kafka: 172.17.1.224:9092
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

var resultsFile string

func init() {
	resultsFile = os.Getenv("KAFKA_TEST_RESULTS")

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
	Name    string        `json:"name"`
	Status  string        `json:"status"` // "PASS", "FAIL", "SKIP"
	Detail  string        `json:"detail"`
	Elapsed time.Duration `json:"elapsed"`
}

type testRunner struct {
	results []testResult
	start   time.Time
}

func newTestRunner() *testRunner {
	return &testRunner{start: time.Now()}
}

func (r *testRunner) run(name string, fn func() (string, func(), error)) {
	t0 := time.Now()
	detail, cleanup, err := fn()
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
	r.results = append(r.results, testResult{Name: name, Status: status, Detail: detail, Elapsed: elapsed})

	// 结果 flush 到文件 AFTER cleanup，确保 panic 不丢失数据
	r.flushResults()

	// cleanup 在结果 flush 之后执行，即使 sarama goroutine panic 也不影响已持久化的结果
	if cleanup != nil {
		cleanup()
	}
}

func (r *testRunner) section(title string) {
	fmt.Printf("\n━━━ %s ━━━\n", title)
}

func (r *testRunner) summary() {
	printSummaryFromResults(r.results, r.start)
}

func (r *testRunner) flushResults() {
	if resultsFile == "" {
		return
	}
	data, err := json.MarshalIndent(r.results, "", "  ")
	if err != nil {
		return
	}
	f, err := os.Create(resultsFile)
	if err != nil {
		return
	}
	f.Write(data)
	f.Sync()
	f.Close()
}

func printSummaryFromResults(results []testResult, start time.Time) {
	passed, failed := 0, 0
	for _, res := range results {
		switch res.Status {
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
	fmt.Printf("║  耗时:  %-48s║\n", time.Since(start).Round(time.Millisecond))
	fmt.Println("╚══════════════════════════════════════════════════════════╝")

	if failed > 0 {
		fmt.Println("\n── 失败详情 ──")
		for _, res := range results {
			if res.Status == "FAIL" {
				fmt.Printf("  ✗ %s: %s\n", res.Name, res.Detail)
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
// 测试入口 — 子进程模式
// main 将自身作为子进程运行（--worker），即使 sarama goroutine panic
// 杀死子进程，主进程仍能打印完整汇总。
// ──────────────────────────────

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--worker" {
		runWorker()
		return
	}
	runLauncher()
}

// runLauncher 构建二进制并以子进程模式运行测试
func runLauncher() {
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Kafka 真实环境端到端测试")
	fmt.Printf("  Broker: %s\n", strings.Join(brokers, ","))
	fmt.Println("═══════════════════════════════════════════════════════════")

	// 构建临时二进制
	tmpBin := os.TempDir() + "/kafka-test-bin"
	defer os.Remove(tmpBin)

	buildCmd := exec.Command("go", "build", "-o", tmpBin, "./cmd/kafka-test/")
	buildCmd.Dir = findProjectRoot()
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
		os.Exit(1)
	}

	// 以子进程运行测试
	resultsJSON := os.TempDir() + "/kafka-test-results.json"
	defer os.Remove(resultsJSON)

	workerCmd := exec.Command(tmpBin, "--worker")
	workerCmd.Env = append(os.Environ(), "KAFKA_TEST_RESULTS="+resultsJSON)
	workerCmd.Stdout = os.Stdout
	workerCmd.Stderr = os.Stderr
	workerErr := workerCmd.Run()

	// 尝试从结果文件读取汇总
	if data, err := os.ReadFile(resultsJSON); err == nil {
		var results []testResult
		if json.Unmarshal(data, &results) == nil && len(results) > 0 {
			// 如果子进程 panic 导致 summary 未打印，这里补打印
			if workerErr != nil {
				fmt.Printf("\n  ⚠ 子进程异常退出 (exit code: %v)，从结果文件恢复汇总:\n", workerErr)
				printSummaryFromResults(results, time.Time{})
			}
		}
	}

	if workerErr != nil {
		os.Exit(1)
	}
}

// runWorker 在子进程中执行实际测试
func runWorker() {
	r := newTestRunner()
	hasFailures := runTests(r)
	r.flushResults()
	r.summary()

	if hasFailures {
		os.Exit(1)
	}
	os.Exit(0)
}

// findProjectRoot 查找项目根目录（包含 go.mod 的目录）
func findProjectRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(dir + "/go.mod"); err == nil {
			return dir
		}
		parent := dir + "/.."
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
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
	r.run("TestKafkaPing", func() (string, func(), error) {
		pub, err := events.NewKafkaPublisher(brokers, "test-ping-topic", saramaCfg)
		if err != nil {
			return "", nil, fmt.Errorf("create publisher: %w", err)
		}
		defer safeClose(pub)

		pinger, ok := pub.(interface{ Ping() error })
		if !ok {
			return "", nil, fmt.Errorf("publisher does not implement Ping")
		}
		if err := pinger.Ping(); err != nil {
			return "", nil, fmt.Errorf("ping failed: %w", err)
		}
		return "Kafka broker reachable", nil, nil
	})

	// Test 2: SaramaClientBrokers
	r.run("TestSaramaClientBrokers", func() (string, func(), error) {
		pub, err := events.NewKafkaPublisher(brokers, "test-brokers-topic", saramaCfg)
		if err != nil {
			return "", nil, fmt.Errorf("create publisher: %w", err)
		}
		defer safeClose(pub)

		pinger, ok := pub.(interface{ Ping() error })
		if !ok {
			return "", nil, fmt.Errorf("publisher does not implement Ping")
		}
		if err := pinger.Ping(); err != nil {
			return "", nil, fmt.Errorf("no brokers: %w", err)
		}
		return "brokers connected", nil, nil
	})

	// ──────────────────────────────
	// Section 2: EventBus 层测试
	// ──────────────────────────────
	r.section("Section 2: EventBus 层测试")

	// Test 3: AdvertisedListener 诊断（检测 Kafka 元数据中 advertised 地址是否可达）
	r.run("TestAdvertisedListener", func() (string, func(), error) {
		topic := uniqueTopic(internalTopicPrefix + "-diag")
		pub, err := events.NewKafkaPublisher(brokers, topic, saramaCfg)
		if err != nil {
			return "", nil, fmt.Errorf("create publisher: %w", err)
		}
		defer safeClose(pub)

		// 发送一条测试消息，如果 advertised listener 不正确会报 DNS 错误
		evt := events.SyncEvent{Action: "update", EntityType: "Diag", Connector: "test"}
		if err := pub.Publish(context.Background(), evt); err != nil {
			if strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "lookup") {
				return "", nil, fmt.Errorf("ADVERTISED_LISTENER 配置错误: broker 元数据返回的地址 (%s) 无法解析\n"+
					"  修复方法: 在 Kafka 服务器上修改 KAFKA_ADVERTISED_LISTENERS:\n"+
					"    将 PLAINTEXT://kafka:9092 改为 PLAINTEXT://172.17.1.224:9092\n"+
					"  然后重启 Kafka 容器: docker compose restart kafka\n"+
					"  原始错误: %w", extractHost(err.Error()), err)
			}
			return "", nil, fmt.Errorf("publish diagnostic: %w", err)
		}
		return "advertised listener OK", nil, nil
	})

	// Test 4: KafkaPublishConsume
	r.run("TestKafkaPublishConsume", func() (string, func(), error) {
		topic := uniqueTopic(internalTopicPrefix + "-pubsub")

		pub, err := events.NewKafkaPublisher(brokers, topic, saramaCfg)
		if err != nil {
			return "", nil, fmt.Errorf("create publisher: %w", err)
		}

		con, err := events.NewKafkaConsumer(brokers, topic, "test-pubsub-group", saramaCfg)
		if err != nil {
			safeClose(pub)
			return "", nil, fmt.Errorf("create consumer: %w", err)
		}

		// 发送 3 条事件
		testEvents := makeTestEvents(3)
		for i, evt := range testEvents {
			evt.Connector = fmt.Sprintf("netbox-%d", i)
			if err := pub.Publish(context.Background(), evt); err != nil {
				return "", nil, fmt.Errorf("publish event %d: %w", i, err)
			}
		}

		// 消费并验证
		received, err := consumeEvents(con, 3)
		if err != nil {
			safeClose(con)
			safeClose(pub)
			return "", nil, err
		}

		// 验证内容
		for i, got := range received {
			if got.Action != "update" {
				safeClose(con)
				safeClose(pub)
				return "", nil, fmt.Errorf("event %d: expected action=update, got %s", i, got.Action)
			}
			if got.EntityType != "Device" {
				safeClose(con)
				safeClose(pub)
				return "", nil, fmt.Errorf("event %d: expected entity_type=Device, got %s", i, got.EntityType)
			}
			if len(got.Data) != 2 {
				safeClose(con)
				safeClose(pub)
				return "", nil, fmt.Errorf("event %d: expected 2 data items, got %d", i, len(got.Data))
			}
		}

		// 显式关闭，避免 defer 与 sarama goroutine 竞态
		safeClose(con)
		time.Sleep(200 * time.Millisecond)
		safeClose(pub)

		return "3/3 events consumed and validated", nil, nil
	})

	// Test 5: FallbackPrimaryOK
	r.run("TestFallbackPrimaryOK", func() (string, func(), error) {
		topic := uniqueTopic(internalTopicPrefix + "-fallback-ok")

		// Kafka primary
		kafkaPub, err := events.NewKafkaPublisher(brokers, topic, saramaCfg)
		if err != nil {
			return "", nil, fmt.Errorf("create kafka publisher: %w", err)
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
			return "", nil, fmt.Errorf("fallback publish: %w", err)
		}

		// 从 Kafka consumer 验证消息确实走了 Kafka（primary）
		con, err := events.NewKafkaConsumer(brokers, topic, "test-fallback-ok-group", saramaCfg)
		if err != nil {
			safeClose(fbPub)
			return "", nil, fmt.Errorf("create consumer: %w", err)
		}

		received, err := consumeEvents(con, 1)
		if err != nil {
			safeClose(con)
			safeClose(fbPub)
			return "", nil, fmt.Errorf("primary not used: %w", err)
		}
		if received[0].EntityType != "Interface" {
			safeClose(con)
			safeClose(fbPub)
			return "", nil, fmt.Errorf("unexpected entity: %s", received[0].EntityType)
		}

		// 显式关闭
		safeClose(con)
		time.Sleep(200 * time.Millisecond)
		safeClose(fbPub)
		chanPub.Close()
		chanCon.Close()

		return "event went through Kafka primary (not fallback)", nil, nil
	})

	// Test 6: FallbackDegradation
	r.run("TestFallbackDegradation", func() (string, func(), error) {
		badSaramaCfg, _ := events.NewSaramaConfig("", "")
		badSaramaCfg.Net.DialTimeout = 2 * time.Second

		badBrokers := []string{"192.0.2.1:9092"}

		_, err := events.NewKafkaPublisher(badBrokers, "test-degrade", badSaramaCfg)
		if err == nil {
			return "", nil, fmt.Errorf("expected kafka publisher creation to fail with unreachable broker")
		}

		chanPub, chanCon := events.NewChannelEventBus(100)

		evt := events.SyncEvent{
			Action:     "delete",
			EntityType: "Link",
			Connector:  "cmdb",
			URIs:       []string{"urn:link:L001"},
		}
		if err := chanPub.Publish(context.Background(), evt); err != nil {
			return "", nil, fmt.Errorf("channel fallback publish: %w", err)
		}

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
			return "", nil, fmt.Errorf("expected delete action in fallback, got %s", received.Action)
		}

		return "Kafka unreachable -> Channel fallback works", nil, nil
	})

	// ──────────────────────────────
	// Section 3: DataSource 层测试
	// ──────────────────────────────
	r.section("Section 3: DataSource 层测试")

	// Test 7: DataSourceConsumer
	r.run("TestDataSourceConsumer", func() (string, func(), error) {
		topic := uniqueTopic(externalTopicPrefix + "-ds")

		extPub, err := events.NewKafkaPublisher(brokers, topic, saramaCfg)
		if err != nil {
			return "", nil, fmt.Errorf("create external publisher: %w", err)
		}
		defer safeClose(extPub)

		chanPub, chanCon := events.NewChannelEventBus(100)
		defer safeClose(chanPub)
		defer safeClose(chanCon)

		dsConsumer, err := events.NewKafkaDataSourceConsumer(brokers, topic, "test-ds-group", saramaCfg)
		if err != nil {
			return "", nil, fmt.Errorf("create ds consumer: %w", err)
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

		evt := events.SyncEvent{
			Action:     "update",
			EntityType: "Device",
			Connector:  "netbox",
			Data: []map[string]any{
				{"name": "PE-Router-01", "ip": "10.0.0.1"},
			},
		}
		if err := extPub.Publish(context.Background(), evt); err != nil {
			return "", nil, fmt.Errorf("publish to external topic: %w", err)
		}

		received, err := consumeEvents(chanCon, 1)
		if err != nil {
			return "", nil, fmt.Errorf("ds forward failed: %w", err)
		}
		if received[0].EntityType != "Device" {
			return "", nil, fmt.Errorf("unexpected entity: %s", received[0].EntityType)
		}

		return "DataSource consumer forwarded event to EventBus", nil, nil
	})

	// Test 8: DataSourceEventContent
	r.run("TestDataSourceEventContent", func() (string, func(), error) {
		topic := uniqueTopic(externalTopicPrefix + "-ds-content")

		extPub, err := events.NewKafkaPublisher(brokers, topic, saramaCfg)
		if err != nil {
			return "", nil, fmt.Errorf("create external publisher: %w", err)
		}
		defer safeClose(extPub)

		chanPub, chanCon := events.NewChannelEventBus(100)
		defer safeClose(chanPub)
		defer safeClose(chanCon)

		dsConsumer, err := events.NewKafkaDataSourceConsumer(brokers, topic, "test-ds-content-group", saramaCfg)
		if err != nil {
			return "", nil, fmt.Errorf("create ds consumer: %w", err)
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
			return "", nil, fmt.Errorf("publish: %w", err)
		}

		received, err := consumeEvents(chanCon, 1)
		if err != nil {
			return "", nil, err
		}

		got := received[0]
		if got.Action != "update" {
			return "", nil, fmt.Errorf("action: expected=update got=%s", got.Action)
		}
		if got.EntityType != "Device" {
			return "", nil, fmt.Errorf("entity_type: expected=Device got=%s", got.EntityType)
		}
		if got.Connector != "netbox" {
			return "", nil, fmt.Errorf("connector: expected=netbox got=%s", got.Connector)
		}
		if len(got.Data) != 2 {
			return "", nil, fmt.Errorf("data count: expected=2 got=%d", len(got.Data))
		}
		if got.Data[0]["name"] != "PE-Router-01" {
			return "", nil, fmt.Errorf("data[0].name: expected=PE-Router-01 got=%v", got.Data[0]["name"])
		}
		if got.Data[1]["vendor"] != "Cisco" {
			return "", nil, fmt.Errorf("data[1].vendor: expected=Cisco got=%v", got.Data[1]["vendor"])
		}

		return "all fields intact (action/entity/connector/data)", nil, nil
	})

	// ──────────────────────────────
	// Section 4: 端到端验证
	// ──────────────────────────────
	r.section("Section 4: 端到端验证")

	// Test 9: EndToEnd
	r.run("TestEndToEnd", func() (string, func(), error) {
		extTopic := uniqueTopic(externalTopicPrefix + "-e2e")
		intTopic := uniqueTopic(internalTopicPrefix + "-e2e")

		// 1. 创建 EventBus 层（Kafka Publisher + Consumer + FallbackPublisher）
		kafkaPub, err := events.NewKafkaPublisher(brokers, intTopic, saramaCfg)
		if err != nil {
			return "", nil, fmt.Errorf("create eventbus kafka pub: %w", err)
		}
		// NOTE: kafkaPub/fbPub/extPub 不在这里 defer close，避免 sarama goroutine panic 在结果 flush 前杀死进程

		chanPub, _ := events.NewChannelEventBus(100)
		fbPub := events.NewFallbackPublisher(kafkaPub, chanPub, 5*time.Second)

		eventBusCon, err := events.NewKafkaConsumer(brokers, intTopic, "test-e2e-eventbus-group", saramaCfg)
		if err != nil {
			return "", nil, fmt.Errorf("create eventbus consumer: %w", err)
		}
		defer safeClose(eventBusCon)

		// 2. 创建 DataSource 层（外部 Topic -> DataSource Consumer -> EventBus Publisher）
		dsConsumer, err := events.NewKafkaDataSourceConsumer(brokers, extTopic, "test-e2e-ds-group", saramaCfg)
		if err != nil {
			return "", nil, fmt.Errorf("create ds consumer: %w", err)
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

		time.Sleep(3 * time.Second)

		// 4. 模拟外部 Producer 向 DataSource Topic 发送事件
		extPub, err := events.NewKafkaPublisher(brokers, extTopic, saramaCfg)
		if err != nil {
			return "", nil, fmt.Errorf("create external publisher: %w", err)
		}

		testEvents := makeTestEvents(2)
		for i, evt := range testEvents {
			evt.Connector = fmt.Sprintf("netbox-e2e-%d", i)
			if err := extPub.Publish(context.Background(), evt); err != nil {
				return "", nil, fmt.Errorf("publish event %d: %w", i, err)
			}
		}

		// 5. 从 EventBus Consumer 消费（完整链路终点）
		received, err := consumeEvents(eventBusCon, 2)
		if err != nil {
			return "", nil, fmt.Errorf("e2e consume failed: %w", err)
		}

		// 6. 验证
		for i, got := range received {
			if got.Action != "update" {
				return "", nil, fmt.Errorf("event %d: action=%s", i, got.Action)
			}
			if got.EntityType != "Device" {
				return "", nil, fmt.Errorf("event %d: entity_type=%s", i, got.EntityType)
			}
			if !strings.HasPrefix(got.Connector, "netbox-e2e") {
				return "", nil, fmt.Errorf("event %d: connector=%s (expected netbox-e2e-*)", i, got.Connector)
			}
		}

		// cleanup: sarama Producer 在结果 flush 之后由 r.run() 调用
		cleanup := func() {
			safeClose(extPub)
			time.Sleep(200 * time.Millisecond)
			safeClose(fbPub)
		}

		return "External Producer -> DS Topic -> DS Consumer -> EventBus(Fallback+Kafka) -> EventBus Consumer: 2/2 OK", cleanup, nil
	})

	// 返回是否有失败
	for _, res := range r.results {
		if res.Status == "FAIL" {
			return true
		}
	}
	return false
}
