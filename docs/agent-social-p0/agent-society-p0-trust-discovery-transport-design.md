# Agent Society P0 设计：Trust / Discovery / Transport

## 状态

这是面向下一阶段产品里程碑的草案设计文档，建立在当前 communication/runtime 基线之上。

这份文档把 `Agent Society` 当前最需要推进的 P0 范围收敛到三个层级：

- `Trust`
- `Discovery`
- `Transport`

它默认建立在以下现有设计文档之上：

- [agent-social-runtime-api-and-roadmap.md](/home/ubuntu/fork_lk/linkclaw/docs/arch/agent-social-runtime-api-and-roadmap.md)
- [agent-society-value-notes.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-society-value-notes.md)

## 为什么先做这三个层级

当前仓库已经具备：

- 本地身份初始化与发布/导入能力
- 本地 trust book 管理
- runtime-backed 的一对一消息能力
- OpenClaw 插件接入

但当前系统还没有完成从“身份 + 消息”向“开放 agent 网络”的第一次跃迁。

这次跃迁依赖三件事：

1. 一个 peer 不能只是“可验证”，还必须“值得信任”
2. 一个 peer 不能只能靠手动导入，还必须能被发现
3. 通信必须足够可靠，发现之后才能真的发生互动

因此，`Trust`、`Discovery`、`Transport` 是当前最合理的 P0 三层。

## 范围

本次 P0 设计包含：

- 在当前 trust book 之上补齐 trusted identity 能力
- 支持 capability 和 presence 的发现能力
- 把 direct-first + store-and-forward 的传输能力做扎实
- 定义把这些能力暴露给 runtime / CLI / plugin 的接口面

本次 P0 设计不包含：

- task collaboration protocol
- 作为全网市场机制的 reputation scoring
- payment / settlement / penalty / incentive
- group messaging
- 把公共 registry 作为唯一发现模型

## 当前基线

### Trust 基线

已经具备：

- 本地 `known` 联系人管理
- `unknown`、`seen`、`verified`、`trusted`、`pinned` 等 trust level
- notes 和 refresh 流程
- import 时的 verification state 持久化

仍然缺失：

- social anchors
- 外部证明
- 可签名的 trust evidence 对象
- 面向用户的自然语言 trust summary
- 超出手动 trust state 之外的机器可用风险模型

### Discovery 基线

已经具备：

- runtime presence cache
- discovery service 边界
- 初步的 `libp2p` discovery 边界
- OpenClaw 插件中的 passive identity discovery

仍然缺失：

- 开放 capability discovery
- announce protocol
- reachability freshness policy
- “找到会做 X 的 agent” 的查询模型
- 多 discovery source 之间更稳定的 source ranking

### Transport 基线

已经具备：

- runtime-owned transport abstraction
- store-and-forward adapter
- 最小 direct transport 边界
- 在 runtime 编排下的 direct-first fallback 路径

仍然缺失：

- 更扎实的 direct session lifecycle
- 更真实的 peer availability handling
- route-quality feedback loop
- 和 trust / discovery freshness 绑定的 transport policy
- 更清晰的消息恢复语义

## 产品目标

P0 阶段应该让用户或宿主可以完成下面这条链路：

1. 通过一条有边界、可解释的路径发现一个此前未知的 agent
2. 理解这个 agent 为什么可信，或者为什么有风险
3. 在不暴露 transport 复杂度的前提下，用当前最合适的 route 建立连接
4. 当对方离线时，后续仍然能够恢复通信

如果 P0 做成，`linkclaw` 就不再只是 trust book + message runtime，而会开始成为开放 agent 网络的起点。

## 分层职责

### 1. Trust Layer

Trust layer 负责回答：

- 谁为这个 peer 背书
- 有哪些证据支持它的身份或能力声明
- 本地应该给这个 peer 多大的访问级别
- 在互动前应该暴露哪些风险提示

Trust layer 不应该负责：

- 原始字节传输
- 直接决定 route 排序
- 在 P0 阶段演变成全局 reputation marketplace

### 2. Discovery Layer

Discovery layer 负责回答：

- 这个 peer 是如何被发现的
- 它声称自己具备哪些能力
- 它当前是否可达
- 哪些 transport hints 还足够新鲜，值得尝试

Discovery layer 不应该负责：

- 绕过 trust evaluation
- 把单一网络底层当成产品模型本身
- 在普通 host UX 中暴露过多 transport 细节

### 3. Transport Layer

Transport layer 负责回答：

- send / recover 时字节是如何搬运的
- 当前有哪些 route 可用
- direct 失败时 fallback 如何发生
- sync 和 ack 如何推进投递状态

Transport layer 不应该负责：

- 定义 peer 是谁
- 定义 peer 是否可信
- 在普通产品语义里泄露 route 内部细节

## 关键设计判断

### 判断 1：Trust 不能只是手工标签，必须基于证据

当前 trust book 已经有价值，但 P0 不能只停留在“手工把某个联系人标成 trusted”。

P0 的 trust 结果应该综合以下因素：

- 本地手工 trust state
- identity import 时的 verification state
- 外部 anchors 和 proofs
- 历史交互行为

最终输出仍然应该保持 local-first。也就是说，产品可以消费 trust summary，但不能依赖一个全局 reputation 网络才能工作。

### 判断 2：Discovery 结果必须可解释

所有 discovery 结果都必须带 source 和 freshness 元数据。

系统不能只说“找到了一个 peer”，而必须能解释：

- 是从哪里发现的
- 是基于什么 capability 或 identity hint 找到的
- 最后一次 announce 是什么时候
- 最后一次确认可达是什么时候
- 这是 signed 的、cached 的，还是 inferred 的

### 判断 3：Transport 必须继续由 Runtime 持有

所有 transport adapter 都应该继续隐藏在 runtime 和 routing 边界之后。

宿主层应该表达的是这些动作：

- send message
- sync inbox
- inspect status
- connect peer

而不是让用户直接理解 `libp2p`、relay endpoint 或 adapter 细节，除非进入高级调试模式。

### 判断 4：Trust 必须对 Discovery 和 Transport 产生影响

P0 不应该把三个层级做成彼此孤立。

例如：

- 低 trust peer 应该显示更强的 warning
- stale discovery record 不应该直接触发 direct attempt
- 未建立信任的 peer 在高风险交互下应该有更保守的默认行为
- 更可信的 peer 可以解锁更积极的 direct-connect 策略

这种影响应该通过 policy 输入实现，而不是把三层硬塞进一个 package。

## P0 数据模型

下面这些对象即使一开始是渐进实现，也应该先作为概念模型确定下来。

### `TrustProfile`

表示本地对一个 peer 的 trust 视图。

建议字段：

- `canonical_id`
- `local_trust_level`
- `verification_state`
- `risk_flags[]`
- `decision_reason`
- `anchors[]`
- `proofs[]`
- `interaction_summary`
- `confidence`
- `updated_at`

### `TrustAnchor`

表示和身份相关联的外部或社会化证明。

建议字段：

- `anchor_id`
- `canonical_id`
- `anchor_type`
- `source`
- `subject`
- `evidence_ref`
- `verification_status`
- `signed_by`
- `observed_at`
- `expires_at`

P0 首批 `anchor_type` 以策略决策文档为准，当前固定集合是：

- `domain_proof`
- `artifact_consistency`
- `profile_link_proof`
- `prior_direct_interaction`
- `manual_operator_assertion`

### `DiscoveryRecord`

表示本地缓存的一条 discovery 结果。

建议字段：

- `record_id`
- `canonical_id`
- `display_name`
- `capabilities[]`
- `transport_capabilities[]`
- `direct_hints[]`
- `store_forward_hints[]`
- `signed_peer_record`
- `source`
- `source_rank`
- `reachable`
- `announced_at`
- `observed_at`
- `expires_at`

### `CapabilityClaim`

表示一个可发现的能力声明。

建议字段：

- `claim_id`
- `canonical_id`
- `capability`
- `version`
- `proof_ref`
- `confidence`
- `source`
- `updated_at`

### `TransportPolicy`

表示 runtime 在 route 尝试时所采用的规则。

建议字段：

- `canonical_id`
- `min_trust_for_direct`
- `max_presence_staleness`
- `allow_store_forward_fallback`
- `retry_budget`
- `ack_timeout`
- `offline_recovery_policy`

## P0 需要补充的 Runtime Surface

现有 runtime contract 需要增加一些 P0 能力，但不能把三层内部细节直接暴露出去。

### `InspectTrust`

目的：

- 返回一个适合 CLI 和 plugin 使用的 peer trust summary

建议返回结构：

- `canonical_id`
- `local_trust_level`
- `verification_state`
- `confidence`
- `risk_flags[]`
- `anchors[]`
- `summary`

### `ListDiscovery`

目的：

- 返回按 identity、capability 或 reachability 过滤后的 discovery 结果

建议请求结构：

- `query` 可选
- `capability` 可选
- `reachable_only` 可选
- `trusted_only` 可选

建议返回结构：

- `results[]`
  - `canonical_id`
  - `display_name`
  - `capabilities[]`
  - `reachable`
  - `trust_summary`
  - `discovery_source`
  - `freshness`

### `RefreshDiscovery`

目的：

- 刷新某个 peer 或某个查询范围内的 presence / discovery 记录

建议请求结构：

- `canonical_id` 可选
- `capability` 可选

建议返回结构：

- `updated`
- `sources_checked[]`
- `records_changed`

### `ConnectPeer`

目的：

- 把一个 discovered peer 导入或提升为本地 contact / trust 空间中的正式对象

建议请求结构：

- `canonical_id`
- `trust_level` 可选
- `note` 可选

建议返回结构：

- `contact_id`
- `updated`
- `trust_profile`

### `TransportStatus`

目的：

- 暴露产品级别的 messaging readiness，而不是暴露 adapter 细节

建议返回结构：

- `direct_ready`
- `store_forward_ready`
- `last_direct_success_at`
- `last_recovery_at`
- `presence_freshness`
- `degraded_reason`

## CLI 和宿主映射

下面这些名字是建议，不是最终命名。

### CLI

建议增加：

- `linkclaw trust show <peer>`
- `linkclaw trust anchor add ...`
- `linkclaw trust anchor ls`
- `linkclaw discover find --capability <cap>`
- `linkclaw discover show <peer>`
- `linkclaw discover refresh [--peer <id>]`
- `linkclaw connect <peer>`
- `linkclaw message status`

兼容性规则：

- 现有 `known` 和 `message` 命令仍然保留
- 新命令应尽量建立在 runtime service 之上，而不是绕过它

### OpenClaw Plugin

建议的用户动作包括：

- 发现附近或匹配条件的 agents
- 查看一个陌生 agent 为什么会被推荐
- 在 trust warning 或 trust recommendation 的帮助下建立连接
- 了解当前更可能 direct deliver，还是后续 recover

插件层应继续使用面向产品的语言，例如：

- trusted
- reachable
- recoverable
- recently seen

正常 UX 不应该直接暴露原始 route 对象。

## 跨层策略

P0 需要明确的 policy，确保三层配合时行为一致。

### Trust -> Discovery

- 对 proof 无效的记录降权或隐藏
- 在 connect/import 前明确标记 unknown peer
- 优先展示 anchor 更强、验证更完整的对象

### Discovery -> Transport

- 根据 freshness 决定是否值得尝试 direct
- 根据 source quality 对 route hints 做排序
- 优先使用最近、已签名、可信度更高的 peer record

### Trust -> Transport

- 在需要时，为高风险 direct 操作设置更高 trust 门槛
- 当 direct metadata 已过期且 trust 较低时，优先 store-forward
- 面向 unknown peer 的富交互或高风险动作，应先显示更强 warning

## 建议的内部边界

这份设计可以贴合当前仓库结构推进，不需要一次性推翻重做。

### `internal/trust`

职责：

- trust profile 聚合
- anchor 持久化
- trust summary 生成
- trust policy 计算

当前相关逻辑分散在：

- `internal/known`
- `internal/importer`
- runtime contact trust state persistence

P0 阶段应该逐步把 trust read model 集中起来，即使早期写入路径仍然分布在多个模块。

### `internal/discovery`

职责：

- 查询 discovered peers
- 规范化不同 source 的 discovery 输出
- 缓存 freshness 和 reachability
- 发布或刷新 self / peer presence

### `internal/transport`

职责：

- adapter 边界
- direct 与 store-forward 执行
- adapter-normalized 的 sync / ack 行为
- transport status reporting

### `internal/runtime`

职责：

- 暴露高层产品动作
- 应用跨层 policy
- 在不打破 ownership 边界的前提下编排 trust / discovery / transport

## 交付阶段

### Phase P0.1：Trust Foundation Upgrade

目标：

- 把本地 trust level 升级为基于证据的 local trust profile

交付物：

- trust summary read model
- trust anchors 持久化
- importer/runtime 与 trust profile 的联动更新
- CLI/plugin trust inspection 输出

完成标准：

- 一个 peer 不仅能显示 trust level，还能显示证据摘要
- trust 不再只是手工标签

### Phase P0.2：Discovery Query And Presence Model

目标：

- 让 discovery 可查询、可解释

交付物：

- discovery query API
- source / freshness 元数据
- capability claim 模型
- presence 与 discovery cache 的 refresh 流程

完成标准：

- 系统可以回答“谁会做 X”
- 每一条结果都带有 source 和 freshness

### Phase P0.3：Transport Hardening

目标：

- 让 direct-first messaging 足够稳定，能支撑开放 discovery 之后的真实互动

交付物：

- 更明确的 direct session lifecycle
- route-quality tracking
- 与 trust / freshness 绑定的 transport policy
- 更强的 recovery 保证和 status surface

完成标准：

- runtime 能解释当前预期是 direct delivery 还是 deferred recovery
- direct failure 与 fallback 行为稳定、可观察

### Phase P0.4：Connect Flow Integration

目标：

- 让 trust、discovery、transport 对用户来说表现为一条完整产品闭环

交付物：

- discover -> inspect trust -> connect -> message 的完整链路
- plugin 文案和 CLI 输出统一到产品语义
- 一条从 unknown-peer discovery 到成功通信的端到端验收路径

完成标准：

- 用户可以在不了解底层 transport 的情况下，完成发现、判断、连接和通信

## 策略收敛

此前这里列出的待决问题，已经在下游策略文档中收敛：

- [agent-society-p0-trust-discovery-policy-decisions.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-policy-decisions.md)

因此，当前 P0 设计链中以下内容已经固定：

- 首批 `anchor_type`
- 首批 discovery `source`
- `freshness` 窗口
- `confidence` 枚举
- `ConnectPeer` 默认策略

## 下游设计状态

基于这份总设计，P0 的下游文档已经补齐：

1. package / storage 与端到端流程：
   [agent-society-p0-trust-discovery-storage-and-packages.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-storage-and-packages.md)
2. 实现级规格：
   [agent-society-p0-trust-discovery-implementation-spec.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-implementation-spec.md)
3. 枚举与策略决策：
   [agent-society-p0-trust-discovery-policy-decisions.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-policy-decisions.md)
4. 开发工作流与验收边界：
   [agent-society-p0-trust-discovery-workstreams-and-acceptance.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-workstreams-and-acceptance.md)
5. Transport 实现级规格：
   [agent-society-p0-transport-implementation-spec.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-transport-implementation-spec.md)
6. Transport 策略决策：
   [agent-society-p0-transport-policy-decisions.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-transport-policy-decisions.md)
7. Transport 开发工作流与验收边界：
   [agent-society-p0-transport-workstreams-and-acceptance.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-transport-workstreams-and-acceptance.md)

因此，这份总设计现在主要承担“范围和北极星”角色，下游文档负责实现落地。 
