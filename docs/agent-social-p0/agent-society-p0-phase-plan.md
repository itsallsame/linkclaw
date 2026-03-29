# Agent Society P0：阶段划分

## 状态

这是 `Agent Society P0` 的阶段计划文档。

它承接：

- [agent-society-p0-trust-discovery-transport-design.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-transport-design.md)
- [agent-society-p0-trust-discovery-workstreams-and-acceptance.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-workstreams-and-acceptance.md)
- [agent-society-p0-transport-workstreams-and-acceptance.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-transport-workstreams-and-acceptance.md)

这份文档的目标不是拆 ticket，而是先把 P0 划成适合 Dalek `focus batch` 推进的阶段。

## 执行结果

这份阶段计划已经完成一轮真实执行。

主开发阶段执行结果：

- `Phase 1` 完成
- `Phase 2` 完成
- `Phase 3` 完成
- `Phase 4` 完成
- `Phase 5` 完成
- `Phase 6` 完成

在主开发完成后，又追加了一轮 review 修正执行：

- `Fix Phase A`：`T12 + T13`
- `Fix Phase B`：`T14 + T15`
- `Fix Phase C`：`T16`

上述修正阶段也已全部完成。

最终结果说明：

- 原始 P0 阶段计划可执行
- `discover -> inspect -> connect -> message -> recover` 链路已由计划态进入实现态
- review 中发现的 `connect` / `source` / `discovery_ready` 偏差已被纳入后续修正阶段

## 为什么要先分阶段

P0 的三个层级是：

- `Trust`
- `Discovery`
- `Transport`

但它们不能一股脑放进一个超长 batch。

更合理的方式是先按依赖关系切阶段，再让 PM 逐阶段推进：

- 每个阶段内部是强相关任务
- 每个阶段结束后，PM 再决定是否进入下一阶段
- 一旦当前阶段出现 `blocked`、设计漂移、merge 中间态，影响范围也被限制在本阶段

## 阶段总览

P0 建议拆成 6 个阶段。

### Phase 1：Foundation

目标：

- 把后续所有实现依赖的底座补齐

范围：

- Trust/Discovery 的 schema 与 store foundation
- Transport 的 route type 与 adapter 边界稳定化

对应 workstream：

- Trust/Discovery `WS1`
- Transport `TWS1`

退出标准：

- 新 schema 可迁移
- 新 store 可读写
- `direct` / `store_forward` / `recovery` 的 adapter 边界稳定

### Phase 2：Fact Writers

目标：

- 让系统开始写新事实，而不是只讨论模型

范围：

- importer 写 `trust_anchors`
- importer 写 `discovery_records`
- importer 写 `capability_claims`
- known trust 写 `trust_events`

对应 workstream：

- Trust/Discovery `WS2`
- Trust/Discovery `WS3`

退出标准：

- import 后能看到 trust/discovery 新事实
- 手工 trust 更新会写 event

### Phase 3：Trust / Discovery Services

目标：

- 把散落事实变成稳定聚合服务

范围：

- `internal/trust`
- `internal/discovery`
- ranking / freshness / summary / confidence

对应 workstream：

- Trust/Discovery `WS4`
- Trust/Discovery `WS5`

退出标准：

- 能按 `canonical_id` 返回 `TrustProfile`
- 能按 capability 返回 `DiscoveryQueryResult`
- 结果带 trust summary 与 freshness

### Phase 4：Transport Runtime Integration

目标：

- 把 transport 真的放进 runtime 编排闭环

范围：

- runtime transport orchestration
- direct-first / store-forward / recovery
- route outcome / transport status surface

对应 workstream：

- Transport `TWS2`
- Transport `TWS3`

退出标准：

- direct 失败能 fallback
- recovery 能 sync + ack
- transport outcome 和 status surface 稳定

### Phase 5：User Surface Integration

目标：

- 把 trust/discovery/transport 能力接到用户入口

范围：

- runtime inspect/discovery/connect
- CLI discover/show/connect/message status
- plugin discover/inspect/connect/message flow

对应 workstream：

- Trust/Discovery `WS6`
- Transport `TWS4`

退出标准：

- CLI 和 plugin 都能走完整入口
- 普通输出使用产品语义，不泄露底层 adapter 细节

### Phase 6：E2E Acceptance

目标：

- 验证 `discover -> inspect -> connect -> message -> recover` 全链路

范围：

- E2E 测试
- blocked / recovery / merge 场景回归
- 验收证据整理

退出标准：

- 在线发送通过
- 离线恢复通过
- 异常状态可观察
- 验收结论可沉淀

## 阶段依赖

阶段依赖是严格串行的：

1. `Phase 1 -> Phase 2`
2. `Phase 2 -> Phase 3`
3. `Phase 3 -> Phase 4`
4. `Phase 4 -> Phase 5`
5. `Phase 5 -> Phase 6`

原因很简单：

- 没有 foundation，就没有事实写入
- 没有事实写入，就没有聚合服务
- 没有聚合服务，transport policy 就没有稳定输入
- 没有 runtime integration，就不该接用户入口
- 没有入口打通，就不该做最终验收

## 每个阶段适合的 focus batch 形态

### Phase 1

建议：

- 小 batch
- 只放底座型 ticket
- 不碰 CLI / plugin

### Phase 2

建议：

- 中等 batch
- importer 和 known trust 可以分开，但仍建议同阶段推进

### Phase 3

建议：

- 中等 batch
- 以 service/read-model 为中心
- 避免过早接 runtime

### Phase 4

建议：

- 小 batch
- 专注 runtime transport 编排
- 不和 UI/CLI 表达混在一起

### Phase 5

建议：

- 小 batch
- 专注 runtime / CLI / plugin 入口
- 这是高冲突阶段，最好单独跑

### Phase 6

建议：

- 验收 batch
- 不再引入大结构改动
- 主要做回归、收口和证据沉淀

## PM 的推进规则

PM 在阶段推进时应遵守：

1. 当前阶段未达到退出标准，不进入下一阶段
2. 当前阶段若出现 `blocked`，先解决 blocked，不强行推进后续阶段
3. 当前阶段若发现设计需要修订，只修本阶段相关文档，不让影响无限扩散
4. 高冲突区域只放在后期阶段集中处理

## 结论

P0 不是一个单一 batch，而是一条阶段化交付链。

当前最合理的划分方式就是：

1. `Foundation`
2. `Fact Writers`
3. `Trust / Discovery Services`
4. `Transport Runtime Integration`
5. `User Surface Integration`
6. `E2E Acceptance`

后续如果要拆 ticket，就应该先按这 6 个阶段来分组，而不是直接把所有 ticket 平铺。 
