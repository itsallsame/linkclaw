# Agent Society：Nostr 可恢复异步消息 Ticket 计划

## 状态

这是 `Nostr 可恢复异步消息` 的 ticket 计划文档。

它承接：

- [agent-society-nostr-recoverable-async-messaging-design.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-nostr-recoverable-async-messaging-design.md)
- [agent-society-nostr-recoverable-async-messaging-phase-plan.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-nostr-recoverable-async-messaging-phase-plan.md)

## Ticket 总览

这一轮建议拆成 10 张主开发 ticket。

这些 ticket 最终会按依赖顺序放入单个 batch，一次性推进。

## Phase 1：Schema And Runtime Store

### N1：Nostr schema and runtime store foundation

- 阶段：Phase 1
- 依赖：无
- 交付：
  - 新 migration
  - `runtime_transport_bindings`
  - `runtime_transport_relays`
  - `runtime_relay_sync_state`
  - `runtime_relay_delivery_attempts`
  - `runtime_recovered_event_observations`
  - store CRUD 与基础测试

### N2：Runtime message status and observation persistence

- 阶段：Phase 1
- 依赖：N1
- 交付：
  - message status 扩展为 recoverable async 语义
  - route/outcome/observation 的持久化整理
  - 基础状态转换测试

## Phase 2：Identity And Import Surfaces

### N3：Card and publish surface add nostr bindings

- 阶段：Phase 2
- 依赖：N1
- 交付：
  - `card export`
  - `publish`
  - `agent-card`
  - Nostr pubkeys / relay URLs / capability 声明

### N4：Inspect and import ingest nostr bindings

- 阶段：Phase 2
- 依赖：N1, N3
- 交付：
  - `inspect`
  - `import`
  - 解析、校验并落库 peer nostr bindings 与 relay hints

## Phase 3：Discovery And Runtime Aggregation

### N5：Discovery/runtime aggregate peer relay and pubkey views

- 阶段：Phase 3
- 依赖：N1, N4
- 交付：
  - peer relay/pubkey 读模型
  - 来源优先级规则
  - runtime 可消费的最终 transport binding 视图

## Phase 4：Nostr Transport And Messaging Core

### N6：Implement nostr transport backend

- 阶段：Phase 4
- 依赖：N1, N5
- 交付：
  - `transport/nostr` 真正实现
  - 第三方 relay publish/pull 基础能力
  - transport 层测试

### N7：Direct-first fallback to nostr recoverable async send

- 阶段：Phase 4
- 依赖：N2, N5, N6
- 交付：
  - direct-first
  - direct fail -> relay queue
  - 至少一个 relay 成功即 recoverable
  - 多 relay 并行写

### N8：Sync, dedupe, binding validation, and recovery state transitions

- 阶段：Phase 4
- 依赖：N2, N5, N6, N7
- 交付：
  - sync 位点推进
  - dedupe
  - sender/receiver binding 校验
  - recovered 状态推进

## Phase 5：CLI / Plugin / Product Surface

### N9：CLI and plugin surfaces for recoverable async messaging

- 阶段：Phase 5
- 依赖：N7, N8
- 交付：
  - CLI send/sync/status 文案与输出
  - plugin background sync / notice
  - 产品语义统一

## Phase 6：Acceptance And Regression

### N10：Acceptance, regression, and evidence

- 阶段：Phase 6
- 依赖：N3, N4, N7, N8, N9
- 交付：
  - unit / integration / contract / e2e
  - direct success 用例
  - recoverable async success 用例
  - relay 全失败用例
  - partial relay success 用例
  - binding reject 用例
  - 验收证据文档

## 单 Batch 组织方式

按你的要求，这一轮采用单个 batch。

建议 batch 顺序为：

1. `N1`
2. `N2`
3. `N3`
4. `N4`
5. `N5`
6. `N6`
7. `N7`
8. `N8`
9. `N9`
10. `N10`

## PM 推进规则

即使是单 batch，也建议遵守以下规则：

1. 当前依赖未完成，不应让后序 ticket 提前进入真正实现
2. 一旦当前 ticket `blocked`，优先处理，不跳过继续向后
3. 高冲突文件只允许单票顺序修改
4. 每张 ticket 的完成定义必须引用本设计文档中的目标、状态机和验收标准

## 当前结论

到这一步，这一轮已经具备：

- 设计稿
- phase plan
- ticket plan

下一步应进入：

- Dalek ticket 创建
- 单 batch 编排
- 全量推进开发
