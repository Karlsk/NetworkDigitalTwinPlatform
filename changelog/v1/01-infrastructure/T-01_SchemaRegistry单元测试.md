# T-01: Schema Registry 单元测试

## 1. 任务概述

为 SchemaRegistry 的 Load/Get/List/Validate 方法编写全面的单元测试，覆盖正常路径和异常路径。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 4: 测试阶段 |
| 预估工时 | 1 天 |
| 前置任务 | I-01, I-02 |
| 交付物 | `internal/schema/registry_test.go`、`internal/schema/validator_test.go` |

## 2. 详细测试用例

### Registry 测试 (`registry_test.go`)

| 用例ID | 测试内容 | 期望结果 |
|--------|---------|---------|
| TC-R01 | Load 成功加载所有 YAML | 6 EntityType + 5 RelationType |
| TC-R02 | GetEntityType("Device") | 返回完整结构体 |
| TC-R03 | GetEntityType("NotExist") | 返回 error |
| TC-R04 | GetRelationType("HAS_INTERFACE") | source=[Device], target=[Interface] |
| TC-R05 | ListEntityTypes | 返回 6 个 |
| TC-R06 | ListRelationTypes | 返回 5 个 |
| TC-R07 | Load 交叉校验 | relationFields 引用不存在的 RelationType → Warn 日志 |

### Validator 测试 (`validator_test.go`)

| 用例ID | 测试内容 | 期望结果 |
|--------|---------|---------|
| TC-V01 | required 字段缺失 | 返回 error |
| TC-V02 | enum 非法值 | 返回 error |
| TC-V03 | 默认值填充 | 缺失字段被填充 Default 值 |
| TC-V04 | stableKeys 非空校验 | 空值 → error |
| TC-V05 | 类型校验（string/int） | 类型不匹配 → error |
| TC-V06 | 多字段校验失败 | error 包含所有失败项 |

## 3. 设计原理

- 使用 `testdata/` 目录存放测试用 YAML 文件（与 `ontology/` 分开）
- Table-driven tests 模式，一个 test function 覆盖多个 case
- 不依赖真实 Neo4j，纯内存测试

## 4. 验收标准

- [ ] 所有测试用例通过
- [ ] `go test -v ./internal/schema/...` 无失败
- [ ] 覆盖率 > 80%

## 5. 注意事项

- List 方法返回的切片顺序不确定，测试时断言长度和内容，不断言顺序
- 交叉校验测试需要检查 slog 输出（可用自定义 handler 捕获）
