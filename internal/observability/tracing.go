// Package observability 提供 Prometheus 指标定义与 OpenTelemetry 追踪初始化
package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// TracerName 服务 Tracer 名称。
const TracerName = "network-digital-twin"

// InitTracer 初始化 OTel TracerProvider。
// endpoint: OTLP Collector 地址（如 http://localhost:4318）
// 如果 endpoint 为空，使用 Noop Tracer（不导出，零开销）。
// 返回 *sdktrace.TracerProvider 供调用方 graceful shutdown。
func InitTracer(ctx context.Context, serviceName, endpoint string) (*sdktrace.TracerProvider, error) {
	// 构建资源标签
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	var tp *sdktrace.TracerProvider

	if endpoint == "" {
		// Noop 模式：空 TracerProvider，不导出任何 Span
		tp = sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
		)
	} else {
		// OTLP HTTP Exporter 模式
		exporter, err := otlptracehttp.New(ctx,
			otlptracehttp.WithEndpointURL(endpoint),
		)
		if err != nil {
			return nil, err
		}

		tp = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exporter),
			sdktrace.WithResource(res),
		)
	}

	// 设置全局 TracerProvider
	otel.SetTracerProvider(tp)

	// 设置 W3C TraceContext + Baggage 传播器
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}
