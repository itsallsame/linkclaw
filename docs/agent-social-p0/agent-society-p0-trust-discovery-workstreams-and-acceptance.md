# Agent Society P0：Trust / Discovery 开发工作流与验收边界

## 状态

这是 `Agent Society P0` 面向开发拆分前的边界文档。

它承接：

- [agent-society-p0-trust-discovery-transport-design.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-transport-design.md)
- [agent-society-p0-trust-discovery-storage-and-packages.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-storage-and-packages.md)
- [agent-society-p0-trust-discovery-implementation-spec.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-implementation-spec.md)
- [agent-society-p0-trust-discovery-policy-decisions.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-policy-decisions.md)

这份文档的目标是把设计进一步收敛成“可拆开发任务”的边界说明，但仍然不直接列 ticket。

它回答：

1. P0 应该切成哪几个 workstream
2. 每个 workstream 的 ownership 是什么
3. 哪些可以并行，哪些必须串行
4. 每个 workstream 的完成标准和测试边界是什么

## 1. 使用方式

这份文档不是产品说明，也不是实现细节正文。

它的用途是：

- 给后续拆 ticket 提供边界
- 给多 worker 并行开发时提供 ownership
- 避免不同实现任务互相覆盖

## 2. P0 拆分原则

P0 的开发拆分遵循下面四条原则：

1. 先拆 source-of-truth 层，再拆 read-model 层，再拆入口层
2. 先拆可稳定并行的模块，再拆强依赖模块
3. 同一个 workstream 尽量只拥有一组相邻文件和一类数据职责
4. 每个 workstream 必须有独立可验证的完成标准

## 3. Workstream 总览

P0 建议拆成六个 workstream。

### WS1：Schema And Store Foundations

目标：

- 补齐 trust/discovery 新表和 store 读写基础

核心内容：

- migration
- `internal/trust/store.go`
- `internal/discovery/store.go`
- 对新表的基础 CRUD 测试

产出性质：

- source-of-truth 基础设施

### WS2：Importer Fact Writers

目标：

- 让 inspect/import 流程开始写 trust/discovery 新事实

核心内容：

- `internal/importer/service.go`
- 写 `trust_anchors`
- 写 `discovery_records`
- 写 `capability_claims`

产出性质：

- 事实写入路径

### WS3：Known Trust Event Flow

目标：

- 让手工 trust 变化进入审计流

核心内容：

- `internal/known/service.go`
- `known trust` 写 `trust_events`
- 如有必要，同步 runtime projection

产出性质：

- trust 变更审计路径

### WS4：Trust Service And Read Models

目标：

- 产出统一的 trust profile / trust summary

核心内容：

- `internal/trust/service.go`
- `internal/trust/types.go`
- `internal/trust/policy.go`
- `internal/trust/summary.go`

产出性质：

- trust 聚合层

### WS5：Discovery Service And Query Models

目标：

- 产出 discovery 查询、show、refresh 能力

核心内容：

- `internal/discovery/service.go`
- `internal/discovery/types.go`
- `internal/discovery/ranking.go`
- `internal/discovery/freshness.go`

产出性质：

- discovery 聚合层

### WS6：Runtime / CLI / Plugin Integration

目标：

- 把 trust/discovery 能力接到用户可见入口

核心内容：

- `internal/runtime`
- `internal/cli`
- OpenClaw plugin bridge / command surface

产出性质：

- 用户入口层

## 4. Workstream Ownership

## 4.1 WS1：Schema And Store Foundations

拥有文件：

- `internal/migrate/sql/*`
- `internal/trust/store.go`
- `internal/trust/store_test.go`
- `internal/discovery/store.go`
- `internal/discovery/store_test.go`

拥有数据职责：

- `trust_anchors`
- `trust_events`
- `discovery_records`
- `capability_claims`

不拥有：

- importer 写入逻辑
- trust/discovery 聚合逻辑
- CLI / plugin 接入

## 4.2 WS2：Importer Fact Writers

拥有文件：

- `internal/importer/service.go`
- `internal/importer/service_test.go`

拥有数据职责：

- 把 inspect/import 结果映射成 trust/discovery 事实

不拥有：

- trust summary
- discovery query
- CLI 新命令

## 4.3 WS3：Known Trust Event Flow

拥有文件：

- `internal/known/service.go`
- `internal/known/service_test.go`
- 相关 CLI 命令适配，如仅限 `known trust`

拥有数据职责：

- 手工 trust level 更新
- `trust_events` 写入

不拥有：

- `trust_anchors` 自动写入
- discovery 相关写入

## 4.4 WS4：Trust Service And Read Models

拥有文件：

- `internal/trust/service.go`
- `internal/trust/types.go`
- `internal/trust/policy.go`
- `internal/trust/summary.go`
- 对应测试

拥有数据职责：

- `TrustProfile`
- `TrustSummary`
- anchor 聚合
- confidence 规则

不拥有：

- importer 原始事实采集
- discovery source ranking

## 4.5 WS5：Discovery Service And Query Models

拥有文件：

- `internal/discovery/service.go`
- `internal/discovery/types.go`
- `internal/discovery/ranking.go`
- `internal/discovery/freshness.go`
- 对应测试

拥有数据职责：

- `DiscoveryQueryResult`
- `DiscoveryDetail`
- source rank
- freshness 判定

不拥有：

- trust summary 规则
- connect 最终编排

## 4.6 WS6：Runtime / CLI / Plugin Integration

拥有文件：

- `internal/runtime/*`
- `internal/cli/run.go`
- `internal/cli/*_test.go`
- `openclaw-plugin/src/*`
- `openclaw-plugin/test/*`

拥有数据职责：

- 面向用户的 connect / inspect / discover 入口
- runtime 编排和产品语义输出

不拥有：

- 新表 schema
- trust/discovery 核心聚合规则

## 5. 依赖关系

## 5.1 必须串行的依赖

下面这些依赖必须串行：

1. `WS1 -> WS2`
   原因：没有 schema/store，importer 无法写新事实
2. `WS1 -> WS3`
   原因：没有 `trust_events` 表，手工 trust 变更无法审计
3. `WS1 + WS2 + WS3 -> WS4`
   原因：trust service 聚合需要事实和事件输入
4. `WS1 + WS2 -> WS5`
   原因：discovery service 需要 discovery records 和 capability claims
5. `WS4 + WS5 -> WS6`
   原因：runtime / CLI / plugin 必须建立在聚合服务上

## 5.2 可以并行的依赖

下面这些 workstream 在前置满足后可以并行：

- `WS2` 和 `WS3`
- `WS4` 和 `WS5`
- `WS6` 内部的 CLI 子路径和 plugin 子路径

## 6. 每个 Workstream 的完成标准

## 6.1 WS1 完成标准

必须满足：

- 新 migration 能在干净数据库上成功执行
- 新 migration 能在已有数据库上成功升级
- 四张新表的 store 测试通过
- 不破坏现有 runtime / known / importer 基础测试

最小测试边界：

- migration 测试
- store CRUD 测试
- 索引/唯一键行为测试

## 6.2 WS2 完成标准

必须满足：

- import 成功后可观察到 `discovery_records`
- import 成功后可观察到至少一种 `trust_anchors`
- capability 存在时可观察到 `capability_claims`
- import 的老行为不回退

最小测试边界：

- importer service 单测
- inspect/import 集成测试

## 6.3 WS3 完成标准

必须满足：

- `known trust` 更新后一定写 `trust_events`
- `trust_records` 仍为 source of truth
- 不引入新的 trust level 集合

最小测试边界：

- `known trust` 单测
- 数据落库测试

## 6.4 WS4 完成标准

必须满足：

- 能按 `canonical_id` 读出 `TrustProfile`
- 能稳定输出 `TrustSummary`
- `confidence` 规则与策略文档一致
- `mismatch` 和低信任对象的 summary 明确体现风险

最小测试边界：

- trust service 单测
- 不同 anchor / verification_state / trust_level 组合测试

## 6.5 WS5 完成标准

必须满足：

- 能按 capability 查询 discovery 结果
- 结果附带 trust summary
- source rank 与 freshness 行为符合策略文档
- `Show` 能返回 detail 级输出

最小测试边界：

- discovery service 单测
- ranking/freshness 规则测试
- refresh 流程测试

## 6.6 WS6 完成标准

必须满足：

- runtime 提供 inspect/discovery/connect 入口
- CLI 至少支持 discover/show/connect
- plugin 至少支持 discover/inspect/connect 基础流
- 连接后可继续进入 message send

最小测试边界：

- runtime service 单测
- CLI 命令测试
- plugin 命令测试
- 端到端路径测试

## 7. 文件冲突控制

为了支持多 worker 并行，P0 开发必须按文件 ownership 控制冲突。

### 高冲突区域

- `internal/runtime/*`
- `internal/cli/run.go`
- `internal/importer/service.go`
- `internal/known/service.go`

### 低冲突区域

- `internal/trust/*`
- `internal/discovery/*`
- 新 migration 文件

因此并行顺序应尽量是：

1. 先让一个 worker 做 `WS1`
2. 之后让不同 worker 分开做 `WS2`、`WS3`
3. 再分开做 `WS4`、`WS5`
4. 最后集中做 `WS6`

## 8. 验收顺序

P0 验收不应该只看最终 E2E。

建议按四层验收：

### L1：Schema 验收

- 表存在
- migration 正常
- 基本 CRUD 正常

### L2：事实写入验收

- importer 和 known 能写对数据
- 旧流程不回退

### L3：聚合输出验收

- trust/discovery service 的返回结构稳定
- 策略枚举和规则都落地

### L4：用户入口验收

- CLI / plugin 路径可用
- `discover -> inspect -> connect -> message` 路径打通

## 9. 拆分前最后仍需注意的点

即使有了本文档，后续拆 ticket 时仍然要遵守以下约束：

1. 不要把 WS4 和 WS5 重新混回 runtime
2. 不要让 importer 或 known 再次承担 summary 聚合
3. 不要把 runtime projection 表误当 source-of-truth
4. 不要在 CLI / plugin 入口里直接拼 trust/discovery 规则

## 10. 结论

到这一步，P0 的 trust/discovery 设计已经具备下面这些条件：

- 有总设计
- 有 package / storage 设计
- 有实现级 spec
- 有策略枚举和默认行为
- 有开发 workstream、依赖关系、ownership 和验收边界

也就是说，后续已经可以进入 ticket 级拆分，只是是否现在就拆，取决于你要不要继续先做一次一致性审查。 
