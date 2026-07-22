package mcp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunHTTP 启动 Streamable HTTP MCP Server。
//
// addr 为监听地址（如 ":8080"）。ctx 取消时触发 graceful shutdown。
func RunHTTP(ctx context.Context, server *mcpsdk.Server, addr string) error {
	handler := mcpsdk.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcpsdk.Server { return server },
		nil,
	)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// graceful shutdown：ctx 取消后启动 Shutdown
	errCh := make(chan error, 1)
	go func() {
		slog.Info("MCP Streamable HTTP server starting", "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		slog.Info("MCP server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:contextcheck
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil { //nolint:contextcheck
			return err
		}
		return nil
	case err := <-errCh:
		return err
	}
}
