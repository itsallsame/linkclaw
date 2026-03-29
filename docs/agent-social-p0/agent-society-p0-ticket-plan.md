# Agent Society P0：Ticket 拆分计划

## 状态

这是 `Agent Society P0` 的 ticket 拆分计划文档。

它承接：

- [agent-society-p0-phase-plan.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-phase-plan.md)
- [agent-society-p0-trust-discovery-workstreams-and-acceptance.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-workstreams-and-acceptance.md)
- [agent-society-p0-transport-workstreams-and-acceptance.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-transport-workstreams-and-acceptance.md)

## Ticket 总览

P0 主开发共拆成 11 张 ticket，review 修正阶段又追加了 5 张 ticket。

当前状态：

- `T1-T11` 已完成
- `T12-T16` 已完成

### Phase 1：Foundation

#### T1：Trust/Discovery schema and store foundation

- 阶段：Phase 1
- 依赖：无
- 交付：
  - 新 migration
  - `internal/trust/store.go`
  - `internal/discovery/store.go`
  - 基础 CRUD 测试

#### T2：Transport adapter and route type stabilization

- 阶段：Phase 1
- 依赖：无
- 交付：
  - `direct` / `store_forward` / `recovery` route type 固化
  - transport adapter 边界与测试稳定

### Phase 2：Fact Writers

#### T3：Importer writes trust/discovery facts

- 阶段：Phase 2
- 依赖：T1
- 交付：
  - importer 写 `trust_anchors`
  - importer 写 `discovery_records`
  - importer 写 `capability_claims`

#### T4：Known trust writes trust events

- 阶段：Phase 2
- 依赖：T1
- 交付：
  - `known trust` 写 `trust_events`
  - 对应测试补齐

### Phase 3：Trust / Discovery Services

#### T5：Trust service and trust read models

- 阶段：Phase 3
- 依赖：T1, T3, T4
- 交付：
  - `internal/trust/service.go`
  - `policy.go`
  - `summary.go`
  - `TrustProfile` / `TrustSummary`

#### T6：Discovery service and query models

- 阶段：Phase 3
- 依赖：T1, T3
- 交付：
  - `internal/discovery/service.go`
  - `ranking.go`
  - `freshness.go`
  - `Find` / `Show` / `Refresh`

### Phase 4：Transport Runtime Integration

#### T7：Runtime transport orchestration

- 阶段：Phase 4
- 依赖：T2, T5, T6
- 交付：
  - runtime send/sync/recover 编排稳定
  - direct-first / fallback / recovery 行为对齐策略

#### T8：Transport outcome and status surface

- 阶段：Phase 4
- 依赖：T2, T7
- 交付：
  - route outcome、status、recovery surface
  - 产品级 transport 状态可读

### Phase 5：User Surface Integration

#### T9：Runtime inspect/discovery/connect integration

- 阶段：Phase 5
- 依赖：T5, T6, T7
- 交付：
  - runtime 新接口
  - `InspectTrust`
  - `ListDiscovery`
  - `ConnectPeer`

#### T10：CLI and plugin user surfaces

- 阶段：Phase 5
- 依赖：T8, T9
- 交付：
  - CLI discover/show/connect/message status
  - plugin discover/inspect/connect/message flow

### Phase 6：E2E Acceptance

#### T11：P0 end-to-end acceptance and evidence

- 阶段：Phase 6
- 依赖：T10
- 交付：
  - `discover -> inspect -> connect -> message -> recover`
  - E2E 验收证据

## Review 修正票

### Fix Phase A

#### T12：Connect discovered peer without pre-imported contact

- 阶段：Fix Phase A
- 依赖：T11
- 交付：
  - `connect-peer` 可作用于 discovery record / canonical_id
  - 不再要求预先存在本地 contact

#### T13：Make connect refresh use real discovery resolution

- 阶段：Fix Phase A
- 依赖：T12
- 交付：
  - `connect --refresh` 真正走 discovery refresh/resolve
  - stale 与 fresh presence 行为可区分

### Fix Phase B

#### T14：Unify discovery source enums across code and policy

- 阶段：Fix Phase B
- 依赖：T13
- 交付：
  - discovery source 枚举统一
  - legacy source alias 兼容
  - filter / ranking / output 语义统一

#### T15：Make discovery readiness reflect actual discovery data

- 阶段：Fix Phase B
- 依赖：T14
- 交付：
  - `discovery_ready` 改为基于真实 discovery/presence facts
  - 状态面不再被 transport 开关误导

### Fix Phase C

#### T16：Define connect promotion semantics after discovery

- 阶段：Fix Phase C
- 依赖：T12, T13
- 交付：
  - 明确 connect 后的本地关系提升语义
  - promotion 结果进入 runtime/message/CLI/plugin
  - 规则沉淀到专项文档

## 推荐 batch 组织

### Batch A

- T1
- T2

### Batch B

- T3
- T4

### Batch C

- T5
- T6

### Batch D

- T7
- T8

### Batch E

- T9
- T10

### Batch F

- T11

### Fix Batch A

- T12
- T13

### Fix Batch B

- T14
- T15

### Fix Batch C

- T16

## PM 推进规则

1. 一个 batch 未达到退出标准，不进入下一 batch
2. 若当前 batch 出现 `blocked`，优先处理 inbox，不继续后推
3. 高冲突区域集中在后两批处理
4. ticket 描述里必须带上依赖和验收口径

## 执行结果

本计划已完成两轮执行：

### 主开发执行结果

- `Batch A` 到 `Batch F` 全部完成
- `T1-T11` 全部完成

### Review 修正执行结果

- `Fix Batch A` 到 `Fix Batch C` 全部完成
- `T12-T16` 全部完成

整体结果是：

- P0 主功能实现完成
- review 中识别出的关键偏差也已通过追加 ticket 修正完成
