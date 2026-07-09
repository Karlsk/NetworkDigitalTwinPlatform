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
	// 验证错误码常量符合规范
	assert.Equal(t, 0, CodeSuccess)
	assert.Equal(t, 40001, CodeBadRequest)
	assert.Equal(t, 40101, CodeUnauthorized)
	assert.Equal(t, 40401, CodeNotFound)
	assert.Equal(t, 50101, CodeNotImplemented)
	assert.Equal(t, 42901, CodeRateLimitExceed)
	assert.Equal(t, 50001, CodeInternalError)
	assert.Equal(t, 50301, CodeCircuitBreakOpen)
}
