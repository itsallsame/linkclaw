# Agent Society P0：Trust / Discovery 枚举与策略决策

## 状态

这是 `Agent Society P0` 的策略收敛文档。

它承接：

- [agent-society-p0-trust-discovery-transport-design.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-transport-design.md)
- [agent-society-p0-trust-discovery-storage-and-packages.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-storage-and-packages.md)
- [agent-society-p0-trust-discovery-implementation-spec.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-implementation-spec.md)

它的目标是把实现前还会阻塞 ticket 拆分的“枚举和策略常量”收敛为固定决策。

这份文档回答：

1. P0 固定采用哪些枚举
2. P0 首批支持哪些 `anchor_type`
3. P0 首批允许哪些 discovery source
4. freshness 和 confidence 如何定义
5. `ConnectPeer` 的默认 trust / 风险策略是什么

## 1. 设计原则

P0 的策略定义遵循四条原则：

1. 尽量贴现有仓库已有状态，不另起一套词汇
2. 先覆盖当前仓库真实能采到的数据，不为未来能力提前设计太多空枚举
3. 优先 local-first，避免把全网 reputation 或公共 registry 当成前提
4. 未确定的新能力先放到 `future`，不塞进 P0 首批枚举

## 2. P0 固定枚举

## 2.1 `trust_level`

P0 固定沿用当前仓库已有集合：

- `unknown`
- `seen`
- `verified`
- `trusted`
- `pinned`

语义定义如下：

### `unknown`

- 系统知道这个 peer 的存在
- 但尚未形成足够信任
- 允许 inspect
- 默认不应被包装成“推荐连接对象”

### `seen`

- 用户或系统已经有过一次明确接触或 connect 动作
- 可以作为“已接触对象”展示
- 不等于“已验证”或“可信”

### `verified`

- 该 peer 的身份 surface 已通过当前系统认可的验证路径
- 不等于“长期可信”，但高于 `seen`

### `trusted`

- 用户或策略显式认为该 peer 适合持续互动
- 可以降低部分 warning 强度

### `pinned`

- 用户显式把该 peer 提升为高优先级长期对象
- 这是最高本地 trust 等级

## 2.2 `verification_state`

P0 固定采用并沿用当前 resolver 语义：

- `discovered`
- `resolved`
- `consistent`
- `mismatch`

语义定义如下：

### `discovered`

- 找到了 identity 相关线索
- 但没有完成足够的一致性校验

### `resolved`

- 找到了结构化 identity surface
- 但还没有完全形成跨 artifact 一致性结论

### `consistent`

- 关键 artifacts 之间一致
- 这是 P0 中最适合作为“可连接、可进一步信任”的验证状态

### `mismatch`

- 关键 identity artifacts 存在冲突
- 这是 P0 中必须触发风险提醒的状态

## 2.3 `confidence`

P0 固定采用离散枚举，不使用自由文本：

- `unknown`
- `low`
- `medium`
- `high`

原因：

- 容易在 CLI / plugin / policy 判断里复用
- 避免早期 summary 文本不可比较

使用规则：

- `TrustProfile.confidence`
- `CapabilityClaim.confidence`

都统一使用这四档。

## 2.4 `freshness`

P0 固定采用离散枚举，不直接把时间戳暴露为唯一语义：

- `fresh`
- `stale`
- `expired`

同时保留原始时间字段：

- `observed_at`
- `announced_at`
- `expires_at`

面向产品展示时优先使用：

- `fresh`
- `stale`
- `expired`

面向调试或 `--json` 时再返回原始时间。

## 3. P0 首批 `anchor_type`

P0 首批只支持当前仓库真实能采集、且语义清晰的五类 anchor。

固定集合如下：

- `domain_proof`
- `artifact_consistency`
- `profile_link_proof`
- `prior_direct_interaction`
- `manual_operator_assertion`

## 3.1 `domain_proof`

含义：

- peer 的 domain、`did.json`、`webfinger` 等公开 identity surface 能形成归属证明

来源：

- resolver / importer

P0 地位：

- 首批一等公民 anchor

## 3.2 `artifact_consistency`

含义：

- DID、WebFinger、Agent Card、Profile 等 artifacts 之间形成一致性证明

来源：

- resolver / importer

P0 地位：

- 首批一等公民 anchor

## 3.3 `profile_link_proof`

含义：

- profile 页面中可观察到对 canonical id、相关 identity surface 或外部 identity 的链接证明

来源：

- resolver / importer

P0 地位：

- 首批一等公民 anchor

## 3.4 `prior_direct_interaction`

含义：

- 本地已经与该 peer 发生过成功消息交互

来源：

- message/runtime interaction events

P0 地位：

- 首批一等公民 anchor

## 3.5 `manual_operator_assertion`

含义：

- 用户或 operator 明确手工确认该对象值得信任

来源：

- `known trust`
- 未来 `trust anchor add`

P0 地位：

- 首批一等公民 anchor

## 3.6 明确不纳入 P0 首批的 anchor

以下 anchor 类型暂不进入 P0 首批固定集合：

- `social_account_proof`
- `shared_group_reference`
- `payment_history`
- `third_party_reputation`

原因：

- 当前仓库没有稳定数据来源
- 容易引入空枚举和伪设计

## 4. P0 首批 discovery source

P0 首批只允许五类 source。

固定集合如下：

- `imported_card`
- `resolver_inspection`
- `runtime_observation`
- `passive_host_discovery`
- `libp2p_announce`

## 4.1 `imported_card`

含义：

- 通过 card import 得到的身份和路由线索

特点：

- 结构清晰
- 稳定
- 但不代表实时可达

P0 角色：

- identity/capability 的基础来源

## 4.2 `resolver_inspection`

含义：

- 通过 inspect / importer 对公开 identity surface 做到的解析结果

特点：

- 可验证
- 可解释
- 适合作为 trust 和 discovery 的基础来源

P0 角色：

- identity consistency 和 capability claim 的基础来源

## 4.3 `runtime_observation`

含义：

- runtime 在真实通信、sync、直连尝试过程中观察到的 presence / reachability 事实

特点：

- 最贴近真实可达性
- 生命周期短

P0 角色：

- transport 选择和 freshness 判断的重要来源

## 4.4 `passive_host_discovery`

含义：

- OpenClaw 等宿主在被动场景下发现到的 identity 线索

特点：

- 对用户较自然
- 可信度通常低于 import/inspect

P0 角色：

- discovery 入口来源

## 4.5 `libp2p_announce`

含义：

- 通过 `libp2p` announce 或 peer record 获取到的发现结果

特点：

- 更接近开放网络
- 新鲜度重要
- 可信度不能只靠 source 本身判定

P0 角色：

- direct transport 候选来源

## 4.6 明确不纳入 P0 首批的 source

以下 source 暂不进入 P0 首批：

- `nostr_discovery`
- `public_registry`
- `shared_directory`
- `third_party_marketplace`

原因：

- 当前仓库还没有足够稳定的 adapter 和产品模型

## 5. source rank 固定规则

P0 采用固定 source rank，从高到低如下：

1. `runtime_observation`
2. `resolver_inspection`
3. `imported_card`
4. `libp2p_announce`
5. `passive_host_discovery`

理由：

- `runtime_observation` 最接近本地真实可达性
- `resolver_inspection` 最适合作为可信 identity 事实
- `imported_card` 稳定但可能较旧
- `libp2p_announce` 对 reachability 有价值，但需要 freshness 约束
- `passive_host_discovery` 适合提示，不适合单独作为强信任依据

## 6. freshness 固定窗口

P0 不采用单一全局 freshness 窗口，而按 source 固定。

### `runtime_observation`

- `fresh`: 0 到 30 分钟
- `stale`: 30 分钟到 6 小时
- `expired`: 超过 6 小时

### `libp2p_announce`

- `fresh`: 0 到 15 分钟
- `stale`: 15 分钟到 2 小时
- `expired`: 超过 2 小时

### `resolver_inspection`

- `fresh`: 0 到 24 小时
- `stale`: 24 小时到 7 天
- `expired`: 超过 7 天

### `imported_card`

- `fresh`: 0 到 7 天
- `stale`: 7 天到 30 天
- `expired`: 超过 30 天

### `passive_host_discovery`

- `fresh`: 0 到 6 小时
- `stale`: 6 小时到 24 小时
- `expired`: 超过 24 小时

## 7. confidence 固定判定规则

P0 先采用简单离散规则，不做复杂打分。

### `TrustProfile.confidence`

#### `high`

满足以下任一：

- `trust_level` 为 `trusted` 或 `pinned`
- `verification_state=consistent` 且存在至少 2 类有效 anchor
- 存在 `prior_direct_interaction` 且不存在 `mismatch`

#### `medium`

满足以下任一：

- `trust_level=verified`
- `verification_state=consistent`
- `trust_level=seen` 且存在至少 1 类有效 anchor

#### `low`

满足以下任一：

- `trust_level=seen`
- `verification_state=resolved`
- 只有单一弱来源 discovery 结果

#### `unknown`

满足以下任一：

- `trust_level=unknown`
- `verification_state=discovered`
- 数据不足以形成有效判断

### `CapabilityClaim.confidence`

#### `high`

- claim 来自 `resolver_inspection`
- 且具备可验证 proof_ref 或 artifact consistency 支撑

#### `medium`

- claim 来自 `imported_card`
- 或来自 `libp2p_announce` 但同时有其他来源印证

#### `low`

- claim 仅来自 `passive_host_discovery`
- 或来源单一且缺乏 proof

#### `unknown`

- 无法判断来源可靠性

## 8. `ConnectPeer` 默认策略

这是 P0 中最关键的产品策略之一。

## 8.1 默认 trust level

当用户执行 `ConnectPeer` 且未显式传入 `trust_level` 时：

- 若 `verification_state=consistent`，默认写入 `seen`
- 若 `verification_state=resolved`，默认写入 `seen`
- 若 `verification_state=discovered`，默认保持 `unknown`
- 若 `verification_state=mismatch`，默认保持 `unknown`

也就是说：

- connect 行为本身不自动提升到 `verified` 或 `trusted`
- connect 只表示“建立联系”，不是“完成信任判定”

## 8.2 风险确认策略

以下情况必须要求 `ConfirmRisk=true` 才允许 connect 继续：

1. `verification_state=mismatch`
2. `TrustProfile.confidence=unknown`
3. 唯一 discovery source 是 `passive_host_discovery`
4. 唯一可用 direct hint 已经 `stale` 或 `expired`

以下情况可以默认继续 connect：

1. `verification_state=consistent`
2. 至少存在一个 `domain_proof` 或 `artifact_consistency`
3. `TrustProfile.confidence` 为 `medium` 或 `high`

## 8.3 对 transport 的默认影响

### direct 的默认门槛

P0 中 direct transport 的默认尝试门槛为：

- `verification_state != mismatch`
- 且 discovery freshness 不是 `expired`

如果不满足：

- 不应优先 direct
- 应优先 store-forward
- 如果没有 store-forward，则返回明确风险提示

### store-forward 的默认门槛

P0 中 store-forward 的默认门槛较低：

- 允许 `unknown`
- 允许 `discovered`
- 但仍需记录风险 warning

理由：

- store-forward 风险通常低于 direct attach 类操作
- 更符合“先接触、再建立信任”的产品路径

## 9. CLI / Plugin 展示固定文案语义

P0 中面向用户的输出不应直接暴露复杂评分，而应映射成稳定语义。

### Trust 展示

- `unknown` -> 未建立信任
- `seen` -> 已接触
- `verified` -> 已验证
- `trusted` -> 已信任
- `pinned` -> 重点信任

### Freshness 展示

- `fresh` -> 最近可达
- `stale` -> 可达性较旧
- `expired` -> 可达性已过期

### Confidence 展示

- `unknown` -> 证据不足
- `low` -> 证据较弱
- `medium` -> 证据中等
- `high` -> 证据充分

## 10. P0 明确不做的策略

以下策略明确不进入 P0：

- 复杂 reputation 分数
- 金融或支付历史参与 trust 计算
- 多来源加权机器学习模型
- 公共 marketplace 排名
- 群体背书网络

原因：

- 这些都不是当前仓库真实可交付的最小系统

## 11. 这份文档对后续实现的约束

当进入实现阶段时，应遵守以下约束：

1. 不新增第二套 `trust_level` 枚举
2. 不新增第二套 `verification_state` 枚举
3. `confidence` 和 `freshness` 必须按本文离散枚举实现
4. P0 首批只实现本文列出的 `anchor_type` 和 discovery `source`
5. `ConnectPeer` 默认策略必须按本文实现，除非先修改本文

## 12. 推荐阅读顺序

1. 总设计：
   [agent-society-p0-trust-discovery-transport-design.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-transport-design.md)
2. package / storage 设计：
   [agent-society-p0-trust-discovery-storage-and-packages.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-storage-and-packages.md)
3. 实现级规格：
   [agent-society-p0-trust-discovery-implementation-spec.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-implementation-spec.md)
4. 本文：
   [agent-society-p0-trust-discovery-policy-decisions.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-policy-decisions.md)

当本文确认后，P0 的 trust/discovery 设计就已经接近可直接拆成工程 ticket。 
