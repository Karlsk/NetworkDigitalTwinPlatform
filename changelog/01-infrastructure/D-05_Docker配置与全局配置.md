# D-05: Docker Compose + 全局配置

## 1. 任务概述

搭建开发环境：Docker Compose 启动 Neo4j CE + Go 服务容器，Viper 加载全局配置。确保 Neo4j 可连接、配置可解析，为后续所有模块提供运行基础。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 1: 设计阶段 |
| 预估工时 | 1 天 |
| 前置任务 | D-01 |
| 交付物 | `deploy/docker-compose.yml`、`internal/config/config.go`、`configs/config.yaml` |

## 2. 详细实现步骤

### Step 1: Docker Compose

**文件**: `deploy/docker-compose.yml`

```yaml
services:
  neo4j:
    image: neo4j:2025.03-community
    ports:
      - "7474:7474"   # Browser UI
      - "7687:7687"   # Bolt protocol
    environment:
      NEO4J_AUTH: neo4j/password
      # 允许大量数据导入的内存配置
      NEO4J_server_memory_heap_initial__size: 512m
      NEO4J_server_memory_heap_max__size: 1G
    volumes:
      - neo4j_data:/data
      - neo4j_logs:/logs
    healthcheck:
      test: ["CMD", "cypher-shell", "-u", "neo4j", "-p", "password", "RETURN 1"]
      interval: 10s
      timeout: 5s
      retries: 5

  app:
    build:
      context: ..
      dockerfile: deploy/Dockerfile
    depends_on:
      neo4j:
        condition: service_healthy
    ports:
      - "8080:8080"
    environment:
      NEO4J_URI: bolt://neo4j:7687
      NEO4J_USER: neo4j
      NEO4J_PASSWORD: password
      CONFIG_PATH: configs/config.yaml

volumes:
  neo4j_data:
  neo4j_logs:
```

**文件**: `deploy/Dockerfile`

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.19
COPY --from=builder /server /server
COPY configs/ /configs/
COPY ontology/ /ontology/
CMD ["/server"]
```

### Step 2: 全局配置

**文件**: `internal/config/config.go`

```go
package config

import (
    "github.com/spf13/viper"
)

// Config 服务全局配置
type Config struct {
    Neo4J    Neo4JConfig    `mapstructure:"neo4j"`
    Server   ServerConfig   `mapstructure:"server"`
    Snapshot SnapshotConfig `mapstructure:"snapshot"`
    Schema   SchemaConfig   `mapstructure:"schema"`
    Channel  ChannelConfig  `mapstructure:"channel"`
}

type Neo4JConfig struct {
    URI       string `mapstructure:"uri"`
    User      string `mapstructure:"user"`
    Password  string `mapstructure:"password"`
    DefaultDB string `mapstructure:"default_db"` // "default"
}

type ServerConfig struct {
    Port int `mapstructure:"port"` // 8080
}

type SnapshotConfig struct {
    Dir       string `mapstructure:"dir"`        // YAML 快照存储目录
    MaxActive int    `mapstructure:"max_active"` // Neo4j 最大逻辑 DB 数
}

type SchemaConfig struct {
    OntologyDir string `mapstructure:"ontology_dir"` // "ontology/"
}

type ChannelConfig struct {
    BufferSize int `mapstructure:"buffer_size"` // eventChan 缓冲大小
}

// Load 从 YAML 文件加载配置
func Load(path string) (*Config, error) {
    viper.SetConfigFile(path)
    viper.AutomaticEnv() // 支持环境变量覆盖

    // 设置默认值
    viper.SetDefault("neo4j.default_db", "default")
    viper.SetDefault("server.port", 8080)
    viper.SetDefault("snapshot.dir", "snapshots")
    viper.SetDefault("snapshot.max_active", 5)
    viper.SetDefault("schema.ontology_dir", "ontology")
    viper.SetDefault("channel.buffer_size", 100)

    if err := viper.ReadInConfig(); err != nil {
        return nil, err
    }

    var cfg Config
    if err := viper.Unmarshal(&cfg); err != nil {
        return nil, err
    }

    return &cfg, nil
}
```

### Step 3: 配置文件

**文件**: `configs/config.yaml`

```yaml
neo4j:
  uri: "bolt://localhost:7687"
  user: "neo4j"
  password: "password"
  default_db: "default"

server:
  port: 8080

snapshot:
  dir: "snapshots"
  max_active: 5

schema:
  ontology_dir: "ontology"

channel:
  buffer_size: 100
```

### Step 4: Connector 配置

**文件**: `configs/connectors.yaml`

```yaml
connectors:
  - name: mock-netbox
    type: mock
    config:
      data_dir: testdata/mock_netbox
    entity_types: [Device, Interface]
  - name: mock-cmdb
    type: mock
    config:
      data_dir: testdata/mock_cmdb
    entity_types: [EVPN_Instance, Network_Slice, SRv6_Policy]
```

## 3. 设计原理

- **Neo4j CE 选型**：社区版免费，但不支持多 DB，因此采用 `_db` 属性实现逻辑多 DB
- **Viper 配置管理**：支持 YAML 文件 + 环境变量覆盖，Docker 环境通过环境变量注入敏感信息
- **healthcheck 机制**：确保 app 容器在 Neo4j 完全就绪后才启动，避免连接失败
- **ChannelConfig**：预配置 eventChan 缓冲大小（100），后续 SyncService 使用
- **Dockerfile 多阶段构建**：builder 阶段编译，最终镜像只有 alpine + 二进制，体积小

### 与其他模块的交互

- `Config.Neo4J` → GraphDB Neo4j 实现（I-07）
- `Config.Schema` → SchemaRegistry.Load（I-01）
- `Config.Snapshot` → SnapshotManager（I-16）
- `Config.Channel` → SyncService.eventChan（I-15）

## 4. 验收标准

- [ ] `docker-compose -f deploy/docker-compose.yml up -d` 启动 Neo4j CE，healthcheck 通过
- [ ] Neo4j Browser UI `http://localhost:7474` 可访问
- [ ] `Config.Load("configs/config.yaml")` 正确解析所有配置项
- [ ] 环境变量可覆盖配置文件（如 `NEO4J_URI=bolt://other:7687`）
- [ ] 默认值生效（未配置时使用 SetDefault 的值）
- [ ] `configs/connectors.yaml` 格式正确，可被后续 ConnectorRegistry 解析

## 5. 注意事项

- Neo4j CE 版本建议使用 `neo4j:2025.03-community`（或最新稳定版）
- Docker 容器内 Neo4j 的 Bolt 地址是 `bolt://neo4j:7687`，本地开发用 `bolt://localhost:7687`
- `NEO4J_AUTH: neo4j/password` 格式是 `用户名/密码`，MVP 用简单密码
- Viper 的 `AutomaticEnv()` 让环境变量自动覆盖配置，Docker 部署时不需要改 YAML
- `snapshots/` 目录用于存放 YAML 快照归档文件，应加入 `.gitignore`
- Connector 配置文件 `connectors.yaml` 中 `config.data_dir` 指向 testdata 目录
