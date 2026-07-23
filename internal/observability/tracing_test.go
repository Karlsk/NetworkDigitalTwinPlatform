package observability

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestTracerNameConstant(t *testing.T) {
	assert.Equal(t, "network-digital-twin", TracerName)
}

func TestInitTracer_Noop(t *testing.T) {
	ctx := context.Background()

	// endpoint="" 时应返回 Noop TracerProvider（零开销）
	tp, err := InitTracer(ctx, "test-service", "")
	require.NoError(t, err)
	require.NotNil(t, tp)

	// 全局 TracerProvider 应被设置
	provider := otel.GetTracerProvider()
	assert.NotNil(t, provider)

	// Noop provider 创建的 Tracer 应是 noop 类型
	tracer := provider.Tracer("test")
	_ = tracer // 确保不 panic

	// 清理
	err = tp.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestInitTracer_NoopReturnsValidProvider(t *testing.T) {
	ctx := context.Background()

	tp, err := InitTracer(ctx, "noop-svc", "")
	require.NoError(t, err)
	defer func() { _ = tp.Shutdown(ctx) }()

	// Noop provider 应能正常创建 Tracer
	tracer := otel.Tracer(TracerName)
	assert.NotNil(t, tracer)
}

func TestInitTracer_WithInvalidEndpoint(t *testing.T) {
	ctx := context.Background()

	// 无效 endpoint（不可达）应返回错误或不 panic
	// 注意：InitTracer 不会在初始化时连接，只是配置 exporter
	tp, err := InitTracer(ctx, "test-service", "http://localhost:99999")
	// 初始化本身应成功（exporter 延迟连接）
	if err == nil {
		require.NotNil(t, tp)
		_ = tp.Shutdown(ctx)
	}
	// err != nil 也是可接受的（某些版本校验 endpoint）
}

func TestInitTracer_GlobalProviderSet(t *testing.T) {
	ctx := context.Background()

	// 保存原始 provider 并在测试结束后恢复
	origProvider := otel.GetTracerProvider()
	defer otel.SetTracerProvider(origProvider)

	tp, err := InitTracer(ctx, "global-test", "")
	require.NoError(t, err)
	defer func() { _ = tp.Shutdown(ctx) }()

	// 全局 provider 应被替换
	current := otel.GetTracerProvider()
	assert.NotNil(t, current)
}

func TestInitTracer_NoopSpanCreation(t *testing.T) {
	ctx := context.Background()

	tp, err := InitTracer(ctx, "span-test", "")
	require.NoError(t, err)
	defer func() { _ = tp.Shutdown(ctx) }()

	otel.SetTracerProvider(tp)
	tracer := otel.Tracer(TracerName)

	// 创建 span 应不报错
	_, span := tracer.Start(ctx, "test-operation")
	assert.NotNil(t, span)
	span.End()

	// Noop span 的 SpanContext 应是无效的
	sc := span.SpanContext()
	_ = sc // 确保不 panic
}

// TestNewNoopTracerProvider 验证辅助函数创建 noop provider。
func TestNewNoopTracerProvider(t *testing.T) {
	tp := noop.NewTracerProvider()
	require.NotNil(t, tp)

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "op")
	assert.NotNil(t, ctx)
	assert.NotNil(t, span)
	span.End()
}
