# V2-17: CI/CD 流水线 (GitHub Actions)

**工时**: 1 天
**前置**: V2-15, V2-16
**风险等级**: 低
**Phase**: Phase 4 — 可观测性 + CI/CD

---

## 背景

V1 无 CI/CD 流水线。V2 建立 GitHub Actions 自动化流水线：
- **Lint**: golangci-lint 代码质量检查
- **Test**: 单元测试 + 集成测试（testcontainers）
- **Coverage**: 代码覆盖率报告
- **Build**: Docker 镜像构建 + 推送

---

## 实现步骤

### Step 1: golangci-lint 配置

新建 `.golangci.yml`：

```yaml
run:
  timeout: 5m
  go: "1.22"

linters:
  enable:
    - errcheck        # 未检查的 error 返回值
    - gosimple        # 代码简化建议
    - govet           # go vet 检查
    - ineffassign     # 无效赋值
    - staticcheck     # 静态分析
    - unused          # 未使用的代码
    - gofmt           # 格式化检查
    - goimports       # import 排序
    - misspell        # 拼写错误
    - revive          # golint 替代
    - bodyclose       # HTTP body 未关闭
    - contextcheck    # context 传递检查
    - errorlint       # error 包装检查
    - nilerr          # nil error 检查

linters-settings:
  revive:
    rules:
      - name: exported
        arguments:
          - disableStutteringCheck
  gofmt:
    simplify: true
  misspell:
    locale: US

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - errcheck
        - bodyclose
    - path: mock/
      linters:
        - revive
```

### Step 2: GitHub Actions — CI 流水线

新建 `.github/workflows/ci.yml`：

```yaml
name: CI

on:
  push:
    branches: [main, v2-*]
  pull_request:
    branches: [main]

env:
  GO_VERSION: "1.22"

jobs:
  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest
          args: --timeout=5m

  test:
    name: Unit Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: Cache Go modules
        uses: actions/cache@v4
        with:
          path: |
            ~/.cache/go-build
            ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
          restore-keys: |
            ${{ runner.os }}-go-

      - name: Download dependencies
        run: go mod download

      - name: Run unit tests
        run: go test -v -race -coverprofile=coverage.out ./...

      - name: Upload coverage
        uses: actions/upload-artifact@v4
        with:
          name: coverage-report
          path: coverage.out

  integration-test:
    name: Integration Tests
    runs-on: ubuntu-latest
    needs: [lint, test]
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: Cache Go modules
        uses: actions/cache@v4
        with:
          path: |
            ~/.cache/go-build
            ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}

      - name: Run integration tests
        run: go test -v -tags=integration -timeout=10m ./...
        env:
          TESTCONTAINERS_RYUK_DISABLED: "true"

  build:
    name: Build
    runs-on: ubuntu-latest
    needs: [test]
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: Build binary
        run: |
          CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bin/server ./cmd/server
          CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bin/migrate-data ./cmd/migrate-data

      - name: Upload binary
        uses: actions/upload-artifact@v4
        with:
          name: server-binary
          path: bin/server

  docker:
    name: Docker Build
    runs-on: ubuntu-latest
    needs: [build]
    if: github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Build Docker image
        uses: docker/build-push-action@v5
        with:
          context: .
          push: false
          tags: network-digital-twin:latest
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

### Step 3: Dockerfile 优化

修改 `Dockerfile`（多阶段构建）：

```dockerfile
# Stage 1: Build
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /server ./cmd/server

# Stage 2: Runtime
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

COPY --from=builder /server .
COPY configs/ ./configs/
COPY ontology/ ./ontology/

EXPOSE 8080

ENTRYPOINT ["./server"]
```

### Step 4: Makefile

新建 `Makefile`：

```makefile
.PHONY: lint test test-integration build docker clean

# Lint
lint:
	golangci-lint run --timeout=5m

# Unit tests
test:
	go test -v -race -coverprofile=coverage.out ./...

# Integration tests (需要 Docker)
test-integration:
	go test -v -tags=integration -timeout=10m ./...

# Build
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/server ./cmd/server
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/migrate-data ./cmd/migrate-data

# Docker build
docker:
	docker build -t network-digital-twin:latest .

# Run locally
run:
	go run ./cmd/server

# Clean
clean:
	rm -rf bin/ coverage.out

# Coverage report
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Swagger doc generation
swagger:
	swag init -g cmd/server/main.go -o docs/swagger
```

### Step 5: PR 模板

新建 `.github/pull_request_template.md`：

```markdown
## Description

<!-- 描述本 PR 的变更内容 -->

## Related Issue

<!-- 关联的 Issue 编号 -->

## Changes

- [ ] 新增功能
- [ ] Bug 修复
- [ ] 重构
- [ ] 文档

## Checklist

- [ ] 代码通过 `make lint`
- [ ] 单元测试通过 `make test`
- [ ] 集成测试通过 `make test-integration`（如适用）
- [ ] 新增/修改了测试用例
- [ ] 更新了相关文档
```

### Step 6: 分支保护规则

通过 GitHub Settings 配置：
- `main` 分支：要求 PR Review + CI 通过
- `v2-*` 分支：CI 通过即可

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `.golangci.yml` | 新增 | golangci-lint 配置 |
| `.github/workflows/ci.yml` | 新增 | GitHub Actions CI 流水线 |
| `Dockerfile` | 修改 | 多阶段构建优化 |
| `Makefile` | 新增 | 常用命令快捷入口 |
| `.github/pull_request_template.md` | 新增 | PR 模板 |

---

## 流水线执行流程

```
Push / PR
    │
    ├── lint (golangci-lint)
    │       ↓
    ├── test (unit tests + coverage)
    │       ↓
    ├── integration-test (testcontainers)
    │       ↓
    ├── build (binary)
    │       ↓
    └── docker (仅 main 分支)
```

---

## 注意事项

1. **Go 版本**: CI 环境与本地 `go.mod` 保持一致（Go 1.22+）
2. **testcontainers**: GitHub Actions 的 Ubuntu Runner 已预装 Docker，支持 testcontainers
3. **Ryuk**: 设置 `TESTCONTAINERS_RYUK_DISABLED=true` 避免 Docker-in-Docker 问题
4. **覆盖率**: 初期不设覆盖率门禁，后续逐步提升到 70%+
5. **Docker Push**: 仅 main 分支触发 Docker 构建，后续可配置 Registry 推送
6. **缓存**: Go modules 和 Docker layer 使用 GitHub Actions Cache 加速

---

## 验收标准

- [ ] `.golangci.yml` 配置完整，`make lint` 通过
- [ ] GitHub Actions CI 流水线在 Push/PR 时自动触发
- [ ] Lint job 通过（无 lint 错误）
- [ ] Unit Test job 通过，覆盖率报告生成
- [ ] Integration Test job 通过（testcontainers）
- [ ] Build job 生成 Linux binary
- [ ] Docker job 构建镜像成功（仅 main 分支）
- [ ] `Makefile` 命令可正常使用
