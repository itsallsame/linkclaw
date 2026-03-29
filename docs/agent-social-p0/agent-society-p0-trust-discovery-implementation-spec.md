# Agent Society P0：Trust / Discovery 实现级规格

## 状态

这是面向实现的硬规格文档。

它承接：

- [agent-society-p0-trust-discovery-transport-design.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-transport-design.md)
- [agent-society-p0-trust-discovery-storage-and-packages.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-storage-and-packages.md)

它的目标是把前两份文档里还偏抽象的内容收敛成实现级约束，重点回答：

1. 哪些 package 是权威 owner
2. 哪些表是 source of truth，哪些只是 projection
3. service 接口和 request/response 的最小固定形状是什么
4. 迁移顺序和阶段依赖如何安排

这份文档仍然不是 ticket 拆分，但它应该足够支撑后续拆 ticket。

## 1. 当前基线

当前仓库已经存在的相关基线：

- `trust_records`
  - 本地 trust level、risk flags、verification state
- `interaction_events`
  - 和联系人相关的交互审计
- `runtime_contacts`
  - runtime 视角下的 contact 投影
- `runtime_presence_cache`
  - runtime 视角下的 presence 缓存
- `internal/discovery.Service`
  - 当前只有 `ResolvePeer`、`RefreshPeer`、`PublishSelf`
- `internal/runtime`
  - 负责 message send/sync/status 等编排

当前最大问题不是“没有数据”，而是：

- ownership 不清晰
- source table 和 projection table 混在一起
- service 接口还不足以支撑 discovery query 和 trust inspect

## 2. 硬性设计结论

以下结论在 P0 阶段视为固定设计，不再反复摇摆。

### 2.1 新建 `internal/trust`

P0 必须新建 `internal/trust`。

原因：

- 当前 trust 聚合逻辑不能继续散落在 `known`、`importer`、`runtime`
- 不单独建包，就无法给 CLI / plugin / runtime 提供稳定的 trust read model

### 2.2 `trust_records` 是 trust manual state 的 source of truth

P0 中：

- `trust_records` 是本地 trust level、risk flags、verification state 的权威来源
- `runtime_contacts.trust_state` 只是 runtime projection，不是权威来源

所以：

- 任何手工 trust level 更新必须先写 `trust_records`
- runtime 读取时可以同步投影到 `runtime_contacts`
- 绝不能反向把 `runtime_contacts.trust_state` 当成最终真相

### 2.3 `discovery_records` 是 discovery 事实层，`runtime_presence_cache` 是 runtime 投影视图

P0 中：

- `discovery_records` 负责保存 discovery source 提供的发现事实
- `runtime_presence_cache` 负责保存 runtime 用于 route planning 的运行态 presence

两者不能合并。

关系是：

- discovery source -> `discovery_records`
- `discovery.Service` 聚合后 -> 按需要刷新 `runtime_presence_cache`

### 2.4 `internal/importer` 负责事实写入，不负责最终 read model

`internal/importer` 可以写：

- `contacts`
- `trust_records`
- `trust_anchors`
- `discovery_records`
- `capability_claims`

但它不能负责：

- trust summary 输出
- discovery 聚合结果输出

### 2.5 `internal/runtime` 只编排，不定义 trust/discovery 规则

`internal/runtime` 只能消费：

- `trust.Service`
- `discovery.Service`

不能自己重新拼装：

- trust summary
- capability match
- source ranking
- freshness policy

## 3. Package Ownership

### 3.1 `internal/trust`

权威职责：

- trust profile 聚合
- trust anchor 管理
- trust summary 生成
- trust policy 计算
- trust event 审计

权威输入：

- `contacts`
- `trust_records`
- `trust_anchors`
- `interaction_events`

权威输出：

- `TrustProfile`
- `TrustSummary`
- `[]TrustAnchor`

### 3.2 `internal/discovery`

权威职责：

- discovery record 管理
- capability claim 管理
- discovery query
- source ranking
- freshness 计算
- runtime presence projection

权威输入：

- `discovery_records`
- `capability_claims`
- `runtime_presence_cache`
- `trust.Service` 输出

权威输出：

- `DiscoveryQueryResult`
- `DiscoveryDetail`
- `RefreshResult`

### 3.3 `internal/known`

保留职责：

- 联系人列表和详情
- 手工 trust level 更新入口
- note 管理

非职责：

- trust 聚合
- discovery query
- capability 搜索

### 3.4 `internal/importer`

保留职责：

- inspect/import 输入解析和验证
- 写 discovery/trust 事实

非职责：

- trust/discovery read model

### 3.5 `internal/runtime`

保留职责：

- connect / message / sync 的高层编排
- runtime oriented status surface

非职责：

- trust 规则计算
- discovery ranking
- trust summary 聚合

## 4. 固定数据模型

## 4.1 Trust 领域

### `TrustProfile`

```go
type TrustProfile struct {
    CanonicalID       string
    ContactID         string
    LocalTrustLevel   string
    VerificationState string
    RiskFlags         []string
    DecisionReason    string
    AnchorCount       int
    ProofCount        int
    InteractionCount  int
    Confidence        string
    Summary           string
    UpdatedAt         time.Time
}
```

### `TrustAnchor`

```go
type TrustAnchor struct {
    AnchorID            string
    CanonicalID         string
    ContactID           string
    AnchorType          string
    Source              string
    Subject             string
    EvidenceRef         string
    VerificationStatus  string
    SignedBy            string
    ObservedAt          time.Time
    ExpiresAt           time.Time
    CreatedAt           time.Time
    UpdatedAt           time.Time
}
```

### `TrustSummary`

```go
type TrustSummary struct {
    CanonicalID       string
    LocalTrustLevel   string
    VerificationState string
    Confidence        string
    RiskFlags         []string
    Summary           string
}
```

## 4.2 Discovery 领域

### `DiscoveryRecord`

```go
type DiscoveryRecord struct {
    RecordID               string
    CanonicalID            string
    DisplayName            string
    Source                 string
    SourceRank             int
    Reachable              bool
    TransportCapabilities  []string
    DirectHints            []string
    StoreForwardHints      []string
    SignedPeerRecord       string
    AnnouncedAt            time.Time
    ObservedAt             time.Time
    ExpiresAt              time.Time
    CreatedAt              time.Time
    UpdatedAt              time.Time
}
```

### `CapabilityClaim`

```go
type CapabilityClaim struct {
    ClaimID     string
    CanonicalID string
    Capability  string
    Version     string
    ProofRef    string
    Source      string
    Confidence  string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### `DiscoveryQueryResult`

```go
type DiscoveryQueryResult struct {
    CanonicalID          string
    DisplayName          string
    Capabilities         []string
    Reachable            bool
    Freshness            string
    SourceSummary        string
    BestDirectHint       string
    BestStoreForwardHint string
    TrustSummary         TrustSummary
}
```

## 5. 数据库规格

P0 不删除现有表，只新增。

## 5.1 现有表的定位

### `trust_records`

定位：

- source of truth
- 代表本地 trust 决策状态

### `interaction_events`

定位：

- 审计输入
- trust summary 的辅助数据来源

### `runtime_contacts`

定位：

- runtime projection
- 供 runtime message flow 使用

### `runtime_presence_cache`

定位：

- runtime projection
- 供 route planning / sync 使用

## 5.2 新增表规格

### `trust_anchors`

```sql
CREATE TABLE IF NOT EXISTS trust_anchors (
  anchor_id TEXT PRIMARY KEY,
  canonical_id TEXT NOT NULL,
  contact_id TEXT NOT NULL DEFAULT '',
  anchor_type TEXT NOT NULL,
  source TEXT NOT NULL,
  subject TEXT NOT NULL DEFAULT '',
  evidence_ref TEXT NOT NULL DEFAULT '',
  verification_status TEXT NOT NULL DEFAULT 'unverified',
  signed_by TEXT NOT NULL DEFAULT '',
  observed_at TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_trust_anchors_canonical_id
  ON trust_anchors(canonical_id, updated_at);

CREATE INDEX IF NOT EXISTS idx_trust_anchors_contact_id
  ON trust_anchors(contact_id, updated_at);

CREATE INDEX IF NOT EXISTS idx_trust_anchors_type
  ON trust_anchors(anchor_type, verification_status);
```

### `trust_events`

```sql
CREATE TABLE IF NOT EXISTS trust_events (
  event_id TEXT PRIMARY KEY,
  canonical_id TEXT NOT NULL,
  contact_id TEXT NOT NULL DEFAULT '',
  event_type TEXT NOT NULL,
  source TEXT NOT NULL,
  old_value_json TEXT NOT NULL DEFAULT '{}',
  new_value_json TEXT NOT NULL DEFAULT '{}',
  reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_trust_events_canonical_id
  ON trust_events(canonical_id, created_at);

CREATE INDEX IF NOT EXISTS idx_trust_events_contact_id
  ON trust_events(contact_id, created_at);
```

### `discovery_records`

```sql
CREATE TABLE IF NOT EXISTS discovery_records (
  record_id TEXT PRIMARY KEY,
  canonical_id TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL,
  source_rank INTEGER NOT NULL DEFAULT 0,
  reachable INTEGER NOT NULL DEFAULT 0,
  transport_capabilities_json TEXT NOT NULL DEFAULT '[]',
  direct_hints_json TEXT NOT NULL DEFAULT '[]',
  store_forward_hints_json TEXT NOT NULL DEFAULT '[]',
  signed_peer_record TEXT NOT NULL DEFAULT '',
  announced_at TEXT NOT NULL DEFAULT '',
  observed_at TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_discovery_records_canonical_id
  ON discovery_records(canonical_id, updated_at);

CREATE INDEX IF NOT EXISTS idx_discovery_records_source
  ON discovery_records(source, updated_at);

CREATE INDEX IF NOT EXISTS idx_discovery_records_reachable
  ON discovery_records(reachable, updated_at);
```

### `capability_claims`

```sql
CREATE TABLE IF NOT EXISTS capability_claims (
  claim_id TEXT PRIMARY KEY,
  canonical_id TEXT NOT NULL,
  capability TEXT NOT NULL,
  version TEXT NOT NULL DEFAULT '',
  proof_ref TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL,
  confidence TEXT NOT NULL DEFAULT 'unknown',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_capability_claims_canonical_capability
  ON capability_claims(canonical_id, capability);

CREATE INDEX IF NOT EXISTS idx_capability_claims_capability
  ON capability_claims(capability, updated_at);
```

## 5.3 明确不新增的表

P0 阶段不新增：

- `trust_profiles`
- `discovery_query_results`

原因：

- 这两类更适合作为 service 聚合结果
- 先避免引入第二套 summary cache
- 等 P1 或性能压力明确后，再决定是否持久化 summary projection

## 6. Service 接口规格

以下接口在 P0 阶段应该尽量保持稳定。

## 6.1 `internal/trust.Service`

```go
type Service interface {
    GetProfile(ctx context.Context, canonicalID string) (TrustProfile, error)
    Summarize(ctx context.Context, canonicalID string) (TrustSummary, error)
    ListAnchors(ctx context.Context, canonicalID string) ([]TrustAnchor, error)
    AddAnchor(ctx context.Context, req AddAnchorRequest) (TrustAnchor, error)
    SetTrustLevel(ctx context.Context, req SetTrustLevelRequest) (TrustProfile, error)
}
```

### `AddAnchorRequest`

```go
type AddAnchorRequest struct {
    CanonicalID        string
    ContactID          string
    AnchorType         string
    Source             string
    Subject            string
    EvidenceRef        string
    VerificationStatus string
    SignedBy           string
    ObservedAt         time.Time
    ExpiresAt          time.Time
}
```

### `SetTrustLevelRequest`

```go
type SetTrustLevelRequest struct {
    CanonicalID    string
    ContactID      string
    TrustLevel     string
    RiskFlags      []string
    DecisionReason string
    Source         string
}
```

## 6.2 `internal/discovery.Service`

当前接口必须扩展为：

```go
type Service interface {
    ResolvePeer(ctx context.Context, canonicalID string) (PeerPresenceView, error)
    RefreshPeer(ctx context.Context, canonicalID string) (PeerPresenceView, error)
    PublishSelf(ctx context.Context) error

    Find(ctx context.Context, req FindRequest) ([]DiscoveryQueryResult, error)
    Show(ctx context.Context, canonicalID string) (DiscoveryDetail, error)
    Refresh(ctx context.Context, req RefreshRequest) (RefreshResult, error)
    UpsertRecord(ctx context.Context, req UpsertRecordRequest) error
    UpsertCapability(ctx context.Context, req UpsertCapabilityRequest) error
}
```

### `FindRequest`

```go
type FindRequest struct {
    Query         string
    Capability    string
    ReachableOnly bool
    TrustedOnly   bool
    Limit         int
}
```

### `DiscoveryDetail`

```go
type DiscoveryDetail struct {
    Result              DiscoveryQueryResult
    Records             []DiscoveryRecord
    CapabilityClaims    []CapabilityClaim
    Presence            PeerPresenceView
    Trust               TrustProfile
}
```

### `RefreshRequest`

```go
type RefreshRequest struct {
    CanonicalID string
    Capability  string
    Force       bool
}
```

### `RefreshResult`

```go
type RefreshResult struct {
    Updated        bool
    SourcesChecked []string
    RecordsChanged int
}
```

## 6.3 `internal/runtime.Service`

P0 增量接口：

```go
type InspectTrustRequest struct {
    CanonicalID string
}

type ListDiscoveryRequest struct {
    Query         string
    Capability    string
    ReachableOnly bool
    TrustedOnly   bool
    Limit         int
}

type ConnectPeerRequest struct {
    CanonicalID string
    TrustLevel  string
    Note        string
    ConfirmRisk bool
}
```

建议新增：

```go
InspectTrust(ctx context.Context, req InspectTrustRequest) (trust.TrustProfile, error)
ListDiscovery(ctx context.Context, req ListDiscoveryRequest) ([]discovery.DiscoveryQueryResult, error)
ConnectPeer(ctx context.Context, req ConnectPeerRequest) (ConnectPeerResult, error)
```

### `ConnectPeerResult`

```go
type ConnectPeerResult struct {
    ContactID     string
    Created       bool
    Updated       bool
    TrustProfile  trust.TrustProfile
    Presence      discovery.PeerPresenceView
}
```

## 7. 读写 Ownership 矩阵

| 模块 | 读 | 写 | 禁止 |
| --- | --- | --- | --- |
| `internal/known` | `contacts`, `trust_records`, notes | `trust_records`, notes, `trust_events` | 聚合 trust summary；查询 discovery |
| `internal/importer` | identity surfaces, `contacts` | `contacts`, `trust_records`, `trust_anchors`, `discovery_records`, `capability_claims` | 输出最终 trust/discovery view |
| `internal/trust` | `contacts`, `trust_records`, `trust_anchors`, `interaction_events` | `trust_anchors`, `trust_events` | 写 discovery records |
| `internal/discovery` | `discovery_records`, `capability_claims`, `runtime_presence_cache` | `discovery_records`, `capability_claims`, `runtime_presence_cache` | 直接改 trust level |
| `internal/runtime` | `trust.Service`, `discovery.Service`, runtime store | runtime projection、message flow state | 自己重新聚合 trust/discovery 规则 |

## 8. 流程级约束

以下流程约束在 P0 视为固定。

### 8.1 Import 约束

当 `importer` 成功导入 identity surface 时，必须：

1. 确保存在 `contacts`
2. 确保存在 `trust_records`
3. 至少写入一条 `discovery_records`
4. 若存在可识别 anchor，则写 `trust_anchors`
5. 若存在 capability 声明，则写 `capability_claims`

### 8.2 Known Trust 更新约束

当用户手工执行 trust 更新时，必须：

1. 更新 `trust_records`
2. 记录一条 `trust_events`
3. 允许后续 runtime 投影更新 `runtime_contacts.trust_state`

### 8.3 Discovery Query 约束

`discover find` 的结果必须：

- 同时参考 `discovery_records`、`capability_claims`
- 附带 `trust.Service.Summarize`
- 不直接把 `runtime_presence_cache` 当唯一来源

### 8.4 Connect 约束

`ConnectPeer` 必须：

1. 以 canonical id 为主键语义
2. 若无 contact，则创建 contact
3. 若已有 contact，则返回已存在并进行必要更新
4. 若风险较高且 `ConfirmRisk=false`，则返回明确错误而不是隐式继续

## 9. 实现阶段与依赖

下面是面向开发顺序的依赖图，不是 ticket 清单。

### Stage 1：Schema and Store

内容：

- 新增四张表的 migration
- `internal/trust/store.go`
- `internal/discovery/store.go`

前置依赖：

- 无

后续解锁：

- importer 写新表
- service 聚合开发

### Stage 2：Fact Writers

内容：

- `importer` 写 `trust_anchors`
- `importer` 写 `discovery_records`
- `importer` 写 `capability_claims`
- `known trust` 写 `trust_events`

前置依赖：

- Stage 1

后续解锁：

- trust/discovery service 可基于真实数据工作

### Stage 3：Trust / Discovery Services

内容：

- `trust.Service`
- `discovery.Service.Find/Show/Refresh`
- source ranking
- freshness policy

前置依赖：

- Stage 1
- Stage 2

后续解锁：

- runtime 新接口
- CLI/plugin 入口

### Stage 4：Runtime Integration

内容：

- `runtime.InspectTrust`
- `runtime.ListDiscovery`
- `runtime.ConnectPeer`

前置依赖：

- Stage 3

后续解锁：

- CLI discover/connect
- plugin discover/connect

### Stage 5：User-Facing Surfaces

内容：

- CLI discover/show/connect
- plugin discover/inspect/connect flow

前置依赖：

- Stage 4

后续解锁：

- E2E acceptance

## 10. 阶段完成标准

### Stage 1 DoD

- migration 可执行
- store 有基本测试
- 四张新表可读写

### Stage 2 DoD

- inspect/import 后能看到 `discovery_records`
- inspect/import 后能看到至少一类 `trust_anchors`
- 手工 trust 更新会写 `trust_events`

### Stage 3 DoD

- 能通过 canonical id 读出 `TrustProfile`
- 能按 capability 返回 `DiscoveryQueryResult`
- 结果附带 trust summary 与 freshness

### Stage 4 DoD

- runtime 能提供 inspect/discovery/connect 三类新接口
- connect 流程不会绕过 trust/discovery service

### Stage 5 DoD

- CLI 和 plugin 至少有一条 discover -> inspect -> connect -> message 的通路
- 对未知 peer 的风险确认有明确行为

## 11. 已收敛的策略前提

此前这里列出的未收敛项，已经由策略文档固定：

- [agent-society-p0-trust-discovery-policy-decisions.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-policy-decisions.md)

实现时应直接采用其中的约束：

1. P0 首批 `anchor_type`
2. P0 首批 discovery `source`
3. `confidence` 固定枚举
4. `freshness` 固定窗口
5. `ConnectPeer` 默认 trust / 风险策略

## 12. 本文档的使用方式

这份文档的用途不是给最终用户看，而是给后续设计和实现使用。

推荐使用顺序：

1. 先读总设计：
   [agent-society-p0-trust-discovery-transport-design.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-transport-design.md)
2. 再读 package/storage 设计：
   [agent-society-p0-trust-discovery-storage-and-packages.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-storage-and-packages.md)
3. 最后读本文：
   [agent-society-p0-trust-discovery-implementation-spec.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-implementation-spec.md)

当本文档内容被确认后，后续就可以进入 ticket 级拆分。 
