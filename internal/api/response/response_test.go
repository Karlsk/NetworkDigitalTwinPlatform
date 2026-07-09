package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestOK(t *testing.T) {
	engine := gin.New()
	engine.GET("/test", func(c *gin.Context) {
		OK(c, gin.H{"key": "value"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, CodeSuccess, body.Code)
	assert.Equal(t, "success", body.Message)
	assert.NotNil(t, body.Data)
}

func TestOK_NilData(t *testing.T) {
	engine := gin.New()
	engine.GET("/test", func(c *gin.Context) {
		OK(c, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var raw map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &raw)
	require.NoError(t, err)
	_, hasData := raw["data"]
	assert.False(t, hasData, "nil data should be omitted via omitempty")
}

func TestFail(t *testing.T) {
	engine := gin.New()
	engine.GET("/test", func(c *gin.Context) {
		Fail(c, http.StatusBadRequest, CodeBadRequest, "invalid param")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, CodeBadRequest, body.Code)
	assert.Equal(t, "invalid param", body.Message)
	assert.Nil(t, body.Data)
}

func TestPageOK(t *testing.T) {
	engine := gin.New()
	engine.GET("/test", func(c *gin.Context) {
		PageOK(c, []string{"a", "b", "c"}, 50)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body PageResponse
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, CodeSuccess, body.Code)
	assert.Equal(t, "success", body.Message)
	assert.Equal(t, 50, body.Total)
	assert.NotNil(t, body.Data)
}

func TestNotImplemented(t *testing.T) {
	engine := gin.New()
	engine.GET("/test", func(c *gin.Context) {
		NotImplemented(c, "coming soon")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)

	var body Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, CodeNotImplemented, body.Code)
	assert.Equal(t, "coming soon", body.Message)
}

func TestErrorCodeConstants(t *testing.T) {
	// 验证错误码常量符合 6 位 HHHMMX 规范
	// 通用模块 (MM=00)
	assert.Equal(t, ErrorCode(0), CodeSuccess)
	assert.Equal(t, ErrorCode(400001), CodeBadRequest)
	assert.Equal(t, ErrorCode(401001), CodeUnauthorized)
	assert.Equal(t, ErrorCode(404001), CodeNotFound)
	assert.Equal(t, ErrorCode(429001), CodeRateLimitExceed)
	assert.Equal(t, ErrorCode(500001), CodeInternalError)
	assert.Equal(t, ErrorCode(501001), CodeNotImplemented)
	assert.Equal(t, ErrorCode(503001), CodeCircuitBreakOpen)

	// Sync 模块 (MM=01)
	assert.Equal(t, ErrorCode(400011), CodeSyncUnsupportedAction)
	assert.Equal(t, ErrorCode(503011), CodeSyncQueueFull)

	// Snapshot 模块 (MM=02)
	assert.Equal(t, ErrorCode(400021), CodeSnapshotBadRequest)
	assert.Equal(t, ErrorCode(404021), CodeSnapshotNotFound)

	// Topology 模块 (MM=03)
	assert.Equal(t, ErrorCode(400031), CodeTopologyBadRequest)
	assert.Equal(t, ErrorCode(500031), CodeTopologyQueryFailed)

	// Device 模块 (MM=04)
	assert.Equal(t, ErrorCode(400041), CodeDeviceBadRequest)
	assert.Equal(t, ErrorCode(404041), CodeDeviceNotFound)
	assert.Equal(t, ErrorCode(501041), CodeDeviceUnsupported)

	// Monitor 模块 (MM=05)
	assert.Equal(t, ErrorCode(400051), CodeMonitorBadRequest)
	assert.Equal(t, ErrorCode(404051), CodeMonitorNotFound)
	assert.Equal(t, ErrorCode(501051), CodeMonitorUnsupported)
}
