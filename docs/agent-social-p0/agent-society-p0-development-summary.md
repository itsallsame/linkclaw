# Agent Society P0：开发总结

## 状态

这是 `Agent Society P0` 的结果总结文档。

它承接：

- [agent-society-p0-trust-discovery-transport-design.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-transport-design.md)
- [agent-society-p0-phase-plan.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-phase-plan.md)
- [agent-society-p0-ticket-plan.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-ticket-plan.md)
- [t11-e2e-acceptance-evidence.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/t11-e2e-acceptance-evidence.md)
- [t14-discovery-source-policy.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/t14-discovery-source-policy.md)
- [t16-connect-promotion-semantics.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/t16-connect-promotion-semantics.md)

## 目标回顾

P0 的目标不是进入 `Collaboration` 或 `Value`，而是先把 `Agent Society` 的第一层产品闭环做出来：

- `Trust`
- `Discovery`
- `Transport`

对应用户链路是：

- `discover -> inspect -> connect -> message -> recover`

如果这条链路能成立，`linkclaw` 就不再只是 identity + messaging 的组合，而会成为开放 agent network 的最小底座。

## 本轮实际完成内容

### 1. Trust 层完成度

本轮完成了：

- trust/discovery foundation store 与 migration
- importer 写入 runtime trust facts
- known trust 写 `trust_events`
- `internal/trust` service、policy、summary
- `TrustProfile` / `TrustSummary`

结果是：

- trust 不再只是手工标签
- runtime 和用户面现在可以消费结构化 trust summary

### 2. Discovery 层完成度

本轮完成了：

- discovery store foundation
- importer 写入 runtime discovery facts
- `internal/discovery` query service
- freshness、ranking、source 语义
- discovery source 枚举统一与兼容映射
- discovery readiness 状态修正

结果是：

- discovery 结果现在可查询、可解释、可过滤
- source / freshness / reachability 已进入产品面

### 3. Transport 层完成度

本轮完成了：

- runtime transport orchestration
- direct-first / fallback / recovery 路径整理
- route outcome 与 status surface
- transport-ready / recovery 状态面

结果是：

- transport 不再只是内部细节
- runtime / CLI / plugin 可以用产品语义表达 direct / deferred / recovered 状态

### 4. Connect 闭环修正

这是 review 之后最重要的补救结果。

本轮修正了：

- `connect-peer` 不再只面向预先导入的 contact
- `connect --refresh` 真正走 discovery refresh/resolve
- connect 后会把 peer 提升进本地关系状态
- promotion 语义已经明确并沉淀为规则文档

结果是：

- `discover -> inspect -> connect` 不再停留在“联系人世界”
- connect 开始成为开放发现后的正式动作

## Ticket 完成情况

### 主开发票

以下票已全部完成：

- `T1` Trust/Discovery schema and store foundation
- `T2` Transport adapter and route type stabilization
- `T3` Importer writes trust/discovery facts
- `T4` Known trust writes trust events
- `T5` Trust service and trust read models
- `T6` Discovery service and query models
- `T7` Runtime transport orchestration
- `T8` Transport outcome and status surface
- `T9` Runtime inspect/discovery/connect integration
- `T10` CLI and plugin user surfaces
- `T11` P0 end-to-end acceptance and evidence

### Review 修正票

以下票已全部完成：

- `T12` Connect discovered peer without pre-imported contact
- `T13` Make connect refresh use real discovery resolution
- `T14` Unify discovery source enums across code and policy
- `T15` Make discovery readiness reflect actual discovery data
- `T16` Define connect promotion semantics after discovery

## 验收结论

从本轮的自动化与文档证据看，P0 已经具备：

- 发现 peer
- 查看 trust
- 发起 connect
- 发消息
- 在存在 relay/store-forward mailbox 的前提下恢复通信
- blocked 状态可观察

关键验收证据见：

- [t11-e2e-acceptance-evidence.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/t11-e2e-acceptance-evidence.md)

关键修正规则见：

- [t14-discovery-source-policy.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/t14-discovery-source-policy.md)
- [t16-connect-promotion-semantics.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/t16-connect-promotion-semantics.md)

## 当前结论

P0 范围内，目标已经基本达成。

更准确地说：

- `Trust / Discovery / Transport` 三层已经落地
- `discover -> inspect -> connect -> message -> recover` 最小产品闭环已经成立
- review 中识别出的关键偏差已经通过 `T12-T16` 修正

但这仍然只是 `Agent Society` 的 P0。

当前还没有进入：

- `Collaboration Layer`
- `Value Layer`
- reputation / market 机制

所以现在的定位应是：

- 一个可用的 agent identity + trust + discovery + messaging runtime
- 一个开放 agent network 的第一层底座
