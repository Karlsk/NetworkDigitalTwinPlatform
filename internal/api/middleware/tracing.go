package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"

	"gitlab.com/pml/network-digital-twin/internal/observability"
)

// Tracing 为每个 HTTP 请求创建 OpenTelemetry Span。
// 支持 W3C TraceContext 传播（从请求 Header 提取 parent span）。
func Tracing() gin.HandlerFunc {
	tracer := otel.Tracer(observability.TracerName)
	propagator := otel.GetTextMapPropagator()

	return func(c *gin.Context) {
		// 从请求 Header 提取传播上下文（W3C TraceContext）
		ctx := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))

		spanName := fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(c.Request.Method),
				semconv.URLPathKey.String(c.Request.URL.Path),
				attribute.String("http.route", c.FullPath()),
			),
		)
		defer span.End()

		// 将 span 上下文注入到 gin.Context
		c.Request = c.Request.WithContext(ctx)

		// 执行后续 handler
		c.Next()

		// 记录响应状态码
		status := c.Writer.Status()
		span.SetAttributes(semconv.HTTPResponseStatusCode(status))

		// 记录错误（如果有）
		if len(c.Errors) > 0 {
			span.SetAttributes(attribute.Bool("error", true))
			span.SetAttributes(attribute.String("error.message", c.Errors.String()))
		}
	}
}
