# Agent Society P0：Transport 策略决策

## 状态

这是 `Agent Society P0` 面向 `Transport` 层的策略收敛文档。

它承接：

- [agent-society-p0-transport-implementation-spec.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-transport-implementation-spec.md)
- [agent-society-p0-trust-discovery-policy-decisions.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-policy-decisions.md)

它的目标是固定：

1. P0 认可哪些 transport 形态
2. direct / store-forward / recovery 的默认门槛
3. fallback 和 retry 的默认策略
4. 用户面应看到哪些 transport 语义

## 1. P0 生效的 transport 策略

P0 首批生效的 transport 只有三类：

- `direct`
- `store_forward`
- `recovery`

其中：

- `direct`
  - 首批实现为 `libp2p_direct`
- `store_forward`
  - 首批实现为 runtime-owned store-forward adapter
- `recovery`
  - 首批实现为 runtime 编排下的离线恢复形态

## 2. direct 默认策略

默认前提：

- `verification_state != mismatch`
- discovery freshness 不是 `expired`
- 存在可用 direct hint

优先级：

- direct 是首选
- 但不是强制

以下情况不应优先 direct：

1. direct hint `expired`
2. 唯一 source 是低可信 discovery source
3. `mismatch`
4. routing 已明确判定 direct 失败概率过高

## 3. store-forward 默认策略

store-forward 是 P0 的强制 fallback 能力。

默认规则：

- 当 direct 不可用时优先使用
- 对 `unknown` / `discovered` 对象允许使用
- 对低信任 peer 也允许使用，但应保留 warning

原因：

- 它比 direct attach 风险更低
- 它是 P0 “真正可用网络”成立的基础

## 4. recovery 默认策略

P0 中 recovery 不是新 transport，而是 transport-runtime 的恢复形态。

默认规则：

- 用户重新打开宿主或显式 sync 时触发
- 若 adapter 返回 `AdvancedCursor`，必须执行 `Ack`
- 若 ack 失败，必须记录 outcome，不允许吞掉

## 5. fallback 顺序

P0 默认 fallback 顺序固定为：

1. `direct`
2. `store_forward`
3. `recovery`

解释：

- `recovery` 不是 send 时的主动发送目标
- 它是离线路径在后续时刻的恢复动作

因此在发送时实际顺序是：

1. 尝试 `direct`
2. 若失败，则尝试 `store_forward`

在后续恢复时顺序是：

1. 执行 `recovery`
2. 如需要，推进 `Ack`

## 6. retry 默认策略

P0 中：

- `Retryable=true`
  - runtime 应继续尝试后续 route
- `Retryable=false`
  - runtime 应结束该 route

不采用：

- 复杂指数退避
- 跨多小时自动重试调度

这些属于后续阶段，不属于 P0。

## 7. 用户面 transport 语义

P0 中普通用户不应看到：

- `libp2p_direct`
- mailbox backend 名称
- route target
- cursor

普通用户应看到的语义是：

- 正在直接投递
- 对方离线，将稍后恢复
- 已恢复新消息
- 当前没有可用连接路径

## 8. JSON / Debug 面语义

P0 中 `--json` 或 debug 输出允许看到：

- `selected_route`
- `transport`
- `retryable`
- `routes_used`
- `advanced_cursor`

原因：

- 工程侧需要排障
- 但这些不应进入普通产品语言

## 9. 与 trust/discovery 的策略连接

`Transport` 不自己决定 trust/discovery，但必须遵守它们的结果：

- `mismatch` 时不默认 direct
- freshness `expired` 时不默认 direct
- 低信任对象允许 store-forward，但要有 warning
- 高风险 peer 若只有 direct 路径，connect/message 前应要求明确确认

## 10. P0 明确不做的 transport 策略

以下不进入 P0：

- Nostr 正式交付
- 多 hop relay mesh
- 消息优先级队列
- 复杂 delivery SLA
- always-on 后台自动恢复模式

## 11. 结论

P0 的 transport 策略已经固定为：

- direct-first
- store-forward-required
- recovery-explicit
- debug 可见、普通用户不可见底层 transport 细节

在这个策略下，Transport 已经足够与 `Trust / Discovery` 一起进入可拆分实现阶段。 
