# V2-13: Topology 与 Device REST API 端点

**工时**: 1 天
**前置**: V2-11
**风险等级**: 低
**Phase**: Phase 3 — Gin HTTP API

---

## 背景

V2-12 完成了 Sync/Snapshot 端点。本任务实现剩余的 Topology 和 Device 两组 REST API 端点，
对应 V1 MCP 工具中的 `query_topology`、`query_device_info`、`query_monitor`、`query_topology_live`。

### API 端点清单

| 方法 | 路径 | 说明 | 对应 MCP 工具 |
|------|------|------|---------------|
| `GET` | `/api/v1/topology` | 查询图数据库拓扑 | `query_topology` |
| `GET` | `/api/v1/topology/live` | 查询实时拓扑（直连控制器） | `query_topology_live` |
| `GET` | `/api/v1/device/:connector/:query_type` | 查询设备信息 | `query_device_info` |
| `GET` | `/api/v1/monitor/:connector/:query_type` | 查询监控数据 | `query_monitor` |

---

## 实现步骤

### Step 1: Topology Handler

新建 `internal/api/handlers/topology.go`：

```go
package handlers

import (
    "net/http"
    "strconv"

    "github.com/gin-gonic/gin"

    "gitlab.com/pml/network-digital-twin/internal/service"
)

// TopologyHandler 拓扑查询 handler。
type TopologyHandler struct {
    analysisSvc *service.AnalysisService
    deviceSvc   *service.DeviceService
}

// NewTopologyHandler 创建 TopologyHandler。
func NewTopologyHandler(a *service.AnalysisService, d *service.DeviceService) *TopologyHandler {
    return &TopologyHandler{analysisSvc: a, deviceSvc: d}
}

// QueryTopology 查询图数据库拓扑。
// GET /api/v1/topology?label=Device&limit=100
func (h *TopologyHandler) QueryTopology(c *gin.Context) {
    label := c.DefaultQuery("label", "Device")
    limitStr := c.DefaultQuery("limit", "100")
    limit, _ := strconv.Atoi(limitStr)
    if limit <= 0 {
        limit = 100
    }

    result, err := h.analysisSvc.QueryTopology(c.Request.Context(), label, limit)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "nodes": result.Nodes,
        "count": result.Count,
    })
}

// QueryTopologyLive 查询实时拓扑（直连控制器，不经过 Neo4j）。
// GET /api/v1/topology/live?connector=controller-1
func (h *TopologyHandler) QueryTopologyLive(c *gin.Context) {
    connectorName := c.Query("connector")
    if connectorName == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "connector query param is required"})
        return
    }

    result, err := h.deviceSvc.QueryDeviceInfo(c.Request.Context(), service.DeviceInfoRequest{
        ConnectorName: connectorName,
        QueryType:     "topology",
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, result)
}
```

### Step 2: Device Handler

新建 `internal/api/handlers/device.go`：

```go
package handlers

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"

    "gitlab.com/pml/network-digital-twin/internal/service"
)

// DeviceHandler 设备与监控 handler。
type DeviceHandler struct {
    svc *service.DeviceService
}

// NewDeviceHandler 创建 DeviceHandler。
func NewDeviceHandler(svc *service.DeviceService) *DeviceHandler {
    return &DeviceHandler{svc: svc}
}

// QueryDeviceInfo 查询设备信息。
// GET /api/v1/device/:connector/:query_type?device=xxx
func (h *DeviceHandler) QueryDeviceInfo(c *gin.Context) {
    connectorName := c.Param("connector")
    queryType := c.Param("query_type")
    device := c.Query("device")

    result, err := h.svc.QueryDeviceInfo(c.Request.Context(), service.DeviceInfoRequest{
        ConnectorName: connectorName,
        QueryType:     queryType,
        Device:        device,
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, result)
}

// QueryMonitor 查询监控数据。
// GET /api/v1/monitor/:connector/:query_type?device=xxx&port=xxx&metrics=cpu,memory
func (h *DeviceHandler) QueryMonitor(c *gin.Context) {
    connectorName := c.Param("connector")
    queryType := c.Param("query_type")

    req := service.MonitorRequest{
        ConnectorName: connectorName,
        QueryType:     queryType,
        Device:        c.Query("device"),
        Port:          c.Query("port"),
        VPNID:         c.Query("vpn_id"),
        Tunnel:        c.Query("tunnel"),
        Namespace:     c.Query("namespace"),
        Interval:      c.Query("interval"),
        LogType:       c.Query("log_type"),
    }

    // 解析 metrics（逗号分隔）
    if m := c.Query("metrics"); m != "" {
        for _, s := range splitComma(m) {
            req.Metrics = append(req.Metrics, s)
        }
    }

    // 解析时间
    if v := c.Query("start_time"); v != "" {
        t, err := time.Parse(time.RFC3339, v)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_time format"})
            return
        }
        req.StartTime = t
    }
    if v := c.Query("end_time"); v != "" {
        t, err := time.Parse(time.RFC3339, v)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end_time format"})
            return
        }
        req.EndTime = t
    }

    result, err := h.svc.QueryMonitor(c.Request.Context(), req)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, result)
}

// splitComma 逗号分隔字符串拆分。
func splitComma(s string) []string {
    var result []string
    for _, part := range []byte(s) {
        _ = part // placeholder
    }
    // 使用 strings.Split
    for _, v := range splitByComma(s) {
        if v != "" {
            result = append(result, v)
        }
    }
    return result
}

func splitByComma(s string) []string {
    var parts []string
    start := 0
    for i := 0; i < len(s); i++ {
        if s[i] == ',' {
            parts = append(parts, s[start:i])
            start = i + 1
        }
    }
    parts = append(parts, s[start:])
    return parts
}
```

> **注意**: 实际实现中应使用 `strings.Split(s, ",")` 替代手动拆分。上述代码仅为演示逻辑。

### Step 3: 路由注册更新

修改 `internal/api/server.go`：

```go
// RegisterRoutes 注册全部 API 路由。
func (s *Server) RegisterRoutes(deps *HandlerDeps) {
    // ... V2-12 已注册的路由 ...

    // Topology
    topoH := handlers.NewTopologyHandler(deps.AnalysisSvc, deps.DeviceSvc)
    s.router.GET("/topology", topoH.QueryTopology)
    s.router.GET("/topology/live", topoH.QueryTopologyLive)

    // Device
    deviceH := handlers.NewDeviceHandler(deps.DeviceSvc)
    s.router.GET("/device/:connector/:query_type", deviceH.QueryDeviceInfo)
    s.router.GET("/monitor/:connector/:query_type", deviceH.QueryMonitor)
}
```

### Step 4: 统一错误响应格式

新建 `internal/api/handlers/errors.go`：

```go
package handlers

import "github.com/gin-gonic/gin"

// ErrorResponse 统一错误响应格式。
type ErrorResponse struct {
    Error   string `json:"error"`
    Code    int    `json:"code"`
    Message string `json:"message,omitempty"`
}

// respondError 统一错误响应。
func respondError(c *gin.Context, code int, err error) {
    c.JSON(code, ErrorResponse{
        Error: http.StatusText(code),
        Code:  code,
        Message: err.Error(),
    })
}
```

### Step 5: 单元测试

| 测试 | 验证点 |
|------|--------|
| `TestQueryTopology` | GET /api/v1/topology 返回节点列表 |
| `TestQueryTopologyDefaultParams` | 不传 label/limit 使用默认值 |
| `TestQueryTopologyLive` | GET /api/v1/topology/live 直连控制器 |
| `TestQueryTopologyLiveMissingConnector` | 缺少 connector 参数返回 400 |
| `TestQueryDeviceInfo` | GET /api/v1/device/:connector/:query_type |
| `TestQueryDeviceInfoUnsupported` | Connector 不支持 DeviceOperator 返回错误 |
| `TestQueryMonitor` | GET /api/v1/monitor/:connector/:query_type |
| `TestQueryMonitorWithMetrics` | metrics 逗号分隔解析正确 |
| `TestQueryMonitorInvalidTime` | start_time 格式错误返回 400 |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/api/handlers/topology.go` | 新增 | 图拓扑 + 实时拓扑 handler |
| `internal/api/handlers/topology_test.go` | 新增 | Topology 端点测试 |
| `internal/api/handlers/device.go` | 新增 | 设备信息 + 监控查询 handler |
| `internal/api/handlers/device_test.go` | 新增 | Device/Monitor 端点测试 |
| `internal/api/handlers/errors.go` | 新增 | 统一错误响应格式 |
| `internal/api/server.go` | 修改 | 注册 Topology/Device/Monitor 路由 |

---

## 注意事项

1. **QueryDeviceInfo query_type**: 支持 `config/isis/bgp/vpn_config/route/topology`，由 `DeviceService` 分发到对应 Connector 能力接口
2. **QueryMonitor query_type**: 支持 `device/port/vpn/tunnel/alerts/logs`，同上
3. **Connector 不存在**: 返回 500，错误信息包含 `connector "xxx" not found`
4. **能力不支持**: Connector 未实现 `MonitorQuerier` 或 `DeviceOperator` 时返回明确错误提示
5. **时间格式**: 统一使用 RFC3339 格式 (`2006-01-02T15:04:05Z07:00`)
6. **metrics 参数**: URL Query 中以逗号分隔传递，如 `?metrics=cpu,memory,disk`

---

## 验收标准

- [ ] `GET /api/v1/topology` 返回图数据库拓扑数据
- [ ] `GET /api/v1/topology/live?connector=xxx` 返回实时拓扑
- [ ] `GET /api/v1/device/:connector/:query_type` 返回设备信息
- [ ] `GET /api/v1/monitor/:connector/:query_type` 返回监控数据
- [ ] 错误响应格式统一 (`error` + `code` + `message`)
- [ ] 参数校验覆盖（缺少必填参数返回 400）
- [ ] `go test ./internal/api/...` 全部通过
