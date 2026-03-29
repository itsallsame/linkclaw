# Agent Society P0：Trust / Discovery Storage、Package 与端到端流程设计

## 状态

这是 `Agent Society P0` 的下游工程设计文档。

它承接上游总设计文档：

- [agent-society-p0-trust-discovery-transport-design.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-transport-design.md)
- [agent-society-value-notes.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-society-value-notes.md)

它的目标不是重复解释“为什么要做 P0”，而是回答两件事：

1. `trust/discovery` 在代码和存储层面如何落地
2. `discover -> inspect -> connect -> message` 这条产品链路如何定义为可实现、可验收的流程

## 文档范围

本文档包含两部分：

- Part A：code-facing 的 `trust/discovery` storage + package 设计
- Part B：`discover -> inspect -> connect -> message` 的端到端用户流程规格

本文档不展开：

- payment / settlement / penalty
- collaboration protocol
- 全网 reputation 经济系统
- 群聊、多方协作工作流

## 当前基线

当前仓库已经有下面这些相关能力：

- `internal/known`
  - 本地联系人和 trust level 管理
- `internal/importer`
  - inspect/import 时的身份校验、联系人写入、trust record 写入
- `internal/runtime`
  - runtime orchestration 和 runtime-owned store
- `internal/discovery`
  - discovery service boundary 和 presence 视图
- `runtime_presence_cache`
  - runtime 范围内的 presence 缓存

当前问题也很明确：

- `trust` 数据是存在的，但没有形成统一 read model
- `discovery` 有边界和 cache，但没有形成完整 query model
- `known`、`importer`、`runtime` 各自写一点 trust/discovery 相关数据，但 ownership 不清晰
- 还没有一条明确规定好的 `discover -> inspect -> connect -> message` 产品链路

## 设计目标

这份设计要达到四个目标：

1. 明确 `trust` 和 `discovery` 的 package ownership
2. 明确 `trust` 和 `discovery` 的持久化对象及表结构方向
3. 明确 CLI / plugin / runtime 应该调用哪个 service，而不是直接拼装底层数据
4. 明确一条从发现陌生 agent 到成功发出消息的标准用户流程

---

## Part A：Trust / Discovery 的 Storage + Package 设计

## 1. 总体设计判断

### 1.1 新增 `internal/trust`

`trust` 不应该继续分散在 `internal/known`、`internal/importer`、`internal/runtime` 之间由调用方自行聚合。

建议新增：

- `internal/trust`

它负责：

- trust profile 聚合
- trust anchor 持久化
- trust summary 输出
- trust policy 计算

### 1.2 保留 `internal/discovery`，但升级为完整领域模块

`internal/discovery` 当前已经有边界，但职责还偏薄。

P0 之后它应明确负责：

- discovery record 持久化与查询
- capability claim 持久化与查询
- presence freshness 管理
- source ranking 和结果规范化

### 1.3 `internal/known` 退回到“联系人操作入口”

`internal/known` 不再承担 trust 聚合逻辑。

它更适合负责：

- 面向 CLI 的联系人 CRUD
- trust level 的显式人工修改入口
- note 增删改

它不负责：

- trust summary 聚合
- discovery query
- capability search

### 1.4 `internal/importer` 负责写入，不负责最终聚合

`internal/importer` 仍然负责 inspect/import 时的验证和写入。

但它不应该定义最终的 trust / discovery 视图。

它只负责：

- 解析输入
- 验证 identity surface
- 写入 contact、trust anchors、discovery records 的原始事实

最终给 CLI / plugin / runtime 消费的聚合视图，应该交给 `internal/trust` 和 `internal/discovery`。

### 1.5 `internal/runtime` 只编排，不拥有 trust/discovery 领域规则

`internal/runtime` 继续作为高层 orchestration 层。

它可以调用：

- `trust.Service`
- `discovery.Service`
- `routing.Planner`
- `transport.Transport`

但它不应该自己计算：

- trust summary
- trust decision policy
- discovery source ranking
- capability match 结果

## 2. 建议的 Package 结构

建议新增和调整后的模块结构如下：

```text
internal/
  trust/
    service.go
    store.go
    types.go
    summary.go
    policy.go
    service_test.go
    store_test.go

  discovery/
    service.go
    store.go
    types.go
    ranking.go
    freshness.go
    service_test.go
    store_test.go
    libp2p/
    nostr/
    dht/

  known/
    service.go
    note.go
    trust_command.go

  importer/
    service.go

  runtime/
    service.go
    store.go
```

## 3. 建议的数据对象

### 3.1 Trust 领域对象

#### `TrustProfile`

这是给产品层消费的统一 trust 视图。

建议字段：

- `canonical_id`
- `contact_id`
- `local_trust_level`
- `verification_state`
- `risk_flags[]`
- `decision_reason`
- `anchor_count`
- `proof_count`
- `interaction_count`
- `confidence`
- `summary`
- `updated_at`

#### `TrustAnchor`

这是可追溯的 trust 事实对象。

建议字段：

- `anchor_id`
- `canonical_id`
- `contact_id` 可选
- `anchor_type`
- `source`
- `subject`
- `evidence_ref`
- `verification_status`
- `signed_by`
- `observed_at`
- `expires_at`
- `created_at`
- `updated_at`

#### `TrustEvent`

这是 trust 变化或 trust 事实写入时的审计对象。

建议字段：

- `event_id`
- `canonical_id`
- `contact_id`
- `event_type`
- `source`
- `old_value`
- `new_value`
- `reason`
- `created_at`

### 3.2 Discovery 领域对象

#### `DiscoveryRecord`

表示一个来自某个 source 的发现结果。

建议字段：

- `record_id`
- `canonical_id`
- `display_name`
- `source`
- `source_rank`
- `reachable`
- `transport_capabilities[]`
- `direct_hints[]`
- `store_forward_hints[]`
- `signed_peer_record`
- `announced_at`
- `observed_at`
- `expires_at`
- `created_at`
- `updated_at`

#### `CapabilityClaim`

表示一个 peer 对外声明的可发现能力。

建议字段：

- `claim_id`
- `canonical_id`
- `capability`
- `version`
- `proof_ref`
- `source`
- `confidence`
- `created_at`
- `updated_at`

#### `DiscoveryQueryResult`

这是给产品层消费的 discovery 聚合结果，不一定一比一落表。

建议字段：

- `canonical_id`
- `display_name`
- `capabilities[]`
- `reachable`
- `freshness`
- `trust_summary`
- `source_summary`
- `best_direct_hint`
- `best_store_forward_hint`

## 4. 建议的存储模型

P0 不建议推翻已有 schema，而应采用“新增表 + 渐进迁移”的方式。

### 4.1 建议新增表

#### `trust_anchors`

用途：

- 持久化 identity/social/domain 等 trust anchor

建议列：

- `anchor_id`
- `canonical_id`
- `contact_id`
- `anchor_type`
- `source`
- `subject`
- `evidence_ref`
- `verification_status`
- `signed_by`
- `observed_at`
- `expires_at`
- `created_at`
- `updated_at`

#### `trust_events`

用途：

- 审计 trust level 变化和 anchor/proof 写入

建议列：

- `event_id`
- `canonical_id`
- `contact_id`
- `event_type`
- `source`
- `old_value_json`
- `new_value_json`
- `reason`
- `created_at`

#### `discovery_records`

用途：

- 存原始 discovery 事实

建议列：

- `record_id`
- `canonical_id`
- `display_name`
- `source`
- `source_rank`
- `reachable`
- `transport_capabilities_json`
- `direct_hints_json`
- `store_forward_hints_json`
- `signed_peer_record`
- `announced_at`
- `observed_at`
- `expires_at`
- `created_at`
- `updated_at`

#### `capability_claims`

用途：

- 存 capability discovery 结果

建议列：

- `claim_id`
- `canonical_id`
- `capability`
- `version`
- `proof_ref`
- `source`
- `confidence`
- `created_at`
- `updated_at`

### 4.2 可复用的现有表和数据

P0 不应该重复造轮子。以下现有对象应被复用：

- `contacts`
  - 联系人基础身份信息继续放这里
- `trust_records`
  - 作为 local trust level 和 verification state 的基础来源
- `interaction_events`
  - 作为 interaction summary 的输入来源
- `runtime_presence_cache`
  - 继续保留，作为 runtime-oriented presence cache

### 4.3 `runtime_presence_cache` 与 `discovery_records` 的关系

这两者不应该合并为一张表。

推荐边界如下：

- `runtime_presence_cache`
  - 面向 runtime
  - 关注最近一次可达性、transport hints、sync 状态
- `discovery_records`
  - 面向 discovery
  - 关注来源、声明、rank、freshness、发现事实

关系上可以是：

- discovery 结果写入 `discovery_records`
- runtime 启动或 refresh 时，把适合 runtime 使用的结果投影到 `runtime_presence_cache`

也就是说：

- `discovery_records` 是来源事实层
- `runtime_presence_cache` 是运行态投影视图

## 5. Read / Write Ownership

这是实现里最容易混乱的部分，必须明确。

### 5.1 `internal/importer`

负责写入：

- `contacts`
- `trust_records`
- `trust_anchors` 的一部分
- `discovery_records` 的一部分

不负责：

- 生成最终 trust summary
- 生成 discovery query result

### 5.2 `internal/known`

负责写入：

- `trust_records` 的手工 trust level 更新
- `notes`
- 必要的 `trust_events`

不负责：

- 写 discovery records
- 聚合 trust profile

### 5.3 `internal/discovery`

负责写入：

- `discovery_records`
- `capability_claims`
- 需要时刷新 `runtime_presence_cache`

负责读取并聚合：

- `discovery_records`
- `capability_claims`
- `runtime_presence_cache`
- `trust.Service` 输出的 trust summary

### 5.4 `internal/trust`

负责读取并聚合：

- `trust_records`
- `trust_anchors`
- `interaction_events`
- `contacts`

负责写入：

- `trust_anchors`
- `trust_events`
- 需要时回写 policy 相关 summary cache

### 5.5 `internal/runtime`

只读取聚合结果：

- `trust.Service`
- `discovery.Service`

只在需要时触发：

- discovery refresh
- trust inspection
- connect flow orchestration

## 6. 建议的 Service 接口

### 6.1 `trust.Service`

建议提供：

- `GetProfile(ctx, canonicalID string) (TrustProfile, error)`
- `ListAnchors(ctx, canonicalID string) ([]TrustAnchor, error)`
- `AddAnchor(ctx, req AddAnchorRequest) (TrustAnchor, error)`
- `SetTrustLevel(ctx, req SetTrustLevelRequest) (TrustProfile, error)`
- `Summarize(ctx, canonicalID string) (TrustSummary, error)`

### 6.2 `discovery.Service`

建议提供：

- `Find(ctx, req FindRequest) ([]DiscoveryQueryResult, error)`
- `Show(ctx, canonicalID string) (DiscoveryQueryResult, error)`
- `Refresh(ctx, req RefreshRequest) (RefreshResult, error)`
- `UpsertRecord(ctx, req UpsertRecordRequest) error`
- `UpsertCapability(ctx, req UpsertCapabilityRequest) error`

### 6.3 `runtime.Service`

建议增加：

- `InspectTrust(ctx, canonicalID string) (TrustProfile, error)`
- `ListDiscovery(ctx, req ListDiscoveryRequest) ([]DiscoveryQueryResult, error)`
- `ConnectPeer(ctx, req ConnectPeerRequest) (ConnectPeerResult, error)`

这保证 CLI / plugin 走 runtime 时，不需要理解底层 package 边界。

## 7. 迁移策略

P0 不应该一次性重写现有逻辑。

建议按下面顺序迁移：

### Phase A1：先建表和 store

- 新增 `trust_anchors`
- 新增 `trust_events`
- 新增 `discovery_records`
- 新增 `capability_claims`
- 为 `internal/trust` 和 `internal/discovery` 建立 store

### Phase A2：让 importer 写新事实表

- inspect/import 时开始写 `trust_anchors`
- inspect/import 时开始写 `discovery_records`
- 如果卡片或 peer record 带能力声明，则写 `capability_claims`

### Phase A3：让 known trust 写 trust events

- `known trust` 修改 trust level 时，记录 `trust_events`

### Phase A4：引入 trust/discovery 聚合 service

- CLI / plugin 不再自己拼 trust 信息
- runtime 使用 `trust.Service` 和 `discovery.Service`

### Phase A5：再做体验层命令和流转

- `discover find`
- `discover show`
- `connect`
- plugin 发现与连接入口

---

## Part B：discover -> inspect -> connect -> message 端到端流程规格

## 8. 流程目标

这条流程的目标不是“展示底层协议”，而是让用户完成下面这条自然链路：

1. 发现一个陌生 agent
2. 看懂它是谁、为什么会出现在结果里、是否可信
3. 决定要不要建立连接
4. 建立连接后发出第一条消息
5. 如果对方离线，系统仍然能恢复消息

## 9. 核心参与者

### 用户

最终做决定的人，关心的是：

- 这个 agent 是谁
- 它会做什么
- 它值不值得联系
- 现在能不能联系上

### 宿主

比如 OpenClaw plugin 或 CLI。

职责是：

- 提供入口
- 展示结果
- 承接确认动作

### Runtime

职责是：

- 聚合 trust/discovery 结果
- 协调 connect 和 send
- 决定 direct / fallback / recovery

## 10. 标准用户流程

### Step 1：Discover

用户输入一种 discovery intent，例如：

- 找会做某件事的 agent
- 查看当前发现到的 agents
- 根据一个已知 capability 搜索 peers

CLI 示例：

```bash
linkclaw discover find --capability research.web
```

Plugin 示例：

- “帮我找会做网页检索的 agent”

系统行为：

- 调用 `runtime.ListDiscovery`
- `runtime` 调用 `discovery.Service.Find`
- 返回 discovery result list

列表里每条结果至少要显示：

- 名称或 canonical id
- capability 摘要
- reachable / recently seen 状态
- trust 摘要
- discovery 来源摘要

### Step 2：Inspect

用户点开或指定某一个结果查看详情。

CLI 示例：

```bash
linkclaw discover show did:key:z6Mk...
```

系统行为：

- 调用 `runtime.ListDiscovery` 或 `runtime.InspectTrust`
- 返回：
  - 这个 peer 的能力声明
  - 来源和 freshness
  - trust level
  - trust anchors / verification summary
  - 风险提示
  - 当前可达性和推荐连接方式

Inspect 页必须回答四个问题：

1. 它是谁
2. 我为什么看见它
3. 我凭什么信它
4. 我现在能怎么连它

### Step 3：Connect

用户决定建立连接。

CLI 示例：

```bash
linkclaw connect did:key:z6Mk...
```

系统行为：

- 调用 `runtime.ConnectPeer`
- runtime 检查该 peer 是否已存在 contact
- 若不存在，则创建 contact
- 写入或更新：
  - `contacts`
  - `trust_records`
  - `trust_events`
  - 需要时补写 `discovery_records` -> contact 映射

如果 peer 风险较高，宿主应要求用户确认。

确认文案至少应说明：

- 这是一个未知或低信任 peer
- 当前有哪些证据不足
- 你仍然可以连接，但后续交互要谨慎

### Step 4：Message

连接完成后，用户发送第一条消息。

CLI 示例：

```bash
linkclaw message send did:key:z6Mk... "hello"
```

系统行为：

- `runtime.SendMessage`
- runtime 查 contact
- runtime 查 discovery/presence
- routing 产出 route candidates
- transport 执行 direct-first
- direct 不可用则 fallback 到 store-forward

用户看到的结果应是产品语义，而不是 transport 细节。

推荐输出风格：

- 已发送，正在直接投递
- 已发送，对方离线，将在对方下次上线后恢复
- 无法发送，缺少可用连接路径

### Step 5：Recover

如果对方离线或本地稍后重新打开宿主，系统应执行 sync/recover。

系统行为：

- runtime `Sync`
- transport `Sync/Ack`
- 更新 inbox/thread/read model

用户看到的结果应是：

- 恢复了多少条消息
- 哪些对话已更新
- 是否仍有待恢复项

## 11. 各阶段状态与输出要求

### Discover 阶段

成功输出至少包含：

- `canonical_id`
- `display_name`
- `capabilities[]`
- `reachable`
- `trust_summary`
- `source_summary`

失败时应区分：

- 没有匹配结果
- 当前没有可用 discovery source
- 数据过期，需要 refresh

### Inspect 阶段

成功输出至少包含：

- identity 摘要
- trust 摘要
- anchor / verification 摘要
- reachability 状态
- 推荐连接方式

失败时应区分：

- 找不到该 peer
- discovery 结果已过期
- trust 信息不完整

### Connect 阶段

成功输出至少包含：

- contact 已创建或已更新
- 当前 trust level
- 当前连接建议

失败时应区分：

- identity 无法验证
- 风险过高且用户未确认
- contact 写入失败

### Message 阶段

成功输出至少包含：

- message id
- conversation id
- delivery status
- 是否 direct / deferred recovery

失败时应区分：

- 没有可用 route
- peer 配置不完整
- transport 层失败

## 12. CLI 规格建议

### 命令集合

建议在 P0 内新增：

- `linkclaw discover find --capability <cap>`
- `linkclaw discover show <peer>`
- `linkclaw discover refresh [--peer <id>]`
- `linkclaw trust show <peer>`
- `linkclaw trust anchor add ...`
- `linkclaw connect <peer>`

### 输出风格

CLI 应优先输出产品语义：

- `trusted`
- `verification=verified`
- `reachable=yes`
- `freshness=fresh`
- `delivery=deferred`

不要默认输出：

- 原始 route JSON
- 原始 adapter 名称
- 一堆 transport 内部调试字段

调试信息可以放到：

- `--json`
- `--verbose`

## 13. Plugin 规格建议

### 13.1 发现入口

插件应支持：

- 根据 capability 搜索 agents
- 展示推荐理由
- 展示最近可达状态

### 13.2 检查入口

插件详情页或卡片应展示：

- agent 名称
- capability
- trust level
- 为什么可信 / 为什么要小心
- 最近在线或可达性

### 13.3 连接入口

对低信任 peer，应弹出确认。

确认页至少说明：

- 该 peer 是否已验证
- 是否存在外部 anchors
- 是否建议继续

### 13.4 首条消息入口

连接完成后应允许直接发消息，不应要求用户再跳到另一套复杂流程。

## 14. 端到端验收标准

P0 至少应该具备下面这条验收路径：

1. 用户发起 capability discovery
2. 系统返回至少一个 discovery result
3. 用户查看该 result 的 trust / source / reachability 详情
4. 用户确认 connect
5. 系统创建或更新 contact
6. 用户发送第一条消息
7. 如果对方在线，则 direct 或快速送达
8. 如果对方离线，则 store-forward，之后 recover
9. 用户能在 inbox/thread 中看到最终结果

## 15. 推荐实现顺序

如果从工程推进角度排顺序，建议按下面做：

1. 先建 `internal/trust`、`internal/discovery` 的 store 和 types
2. 新增 `trust_anchors`、`trust_events`、`discovery_records`、`capability_claims`
3. 让 `importer` 和 `known trust` 写新事实表
4. 实现 `trust.Service` 和 `discovery.Service`
5. 在 runtime 中引入 `InspectTrust`、`ListDiscovery`、`ConnectPeer`
6. 再做 CLI / plugin 的 discover、inspect、connect 入口
7. 最后做端到端验收和文案打磨

## 16. 这份设计的完成定义

当下面这些条件同时成立时，这份设计可以认为已经可以指导实现：

- `trust/discovery` 的 package ownership 已经明确
- 新表和旧表的关系已经明确
- importer / known / runtime 各自的读写边界已经明确
- `discover -> inspect -> connect -> message` 的产品流程已经明确
- CLI / plugin 至少有一条共同依赖的 runtime contract

到这个程度，团队就可以从“讨论方向”进入“拆任务实现”。 
