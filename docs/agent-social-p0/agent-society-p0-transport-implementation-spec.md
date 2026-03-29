# Agent Society P0：Transport 实现级规格

## 状态

这是 `Agent Society P0` 面向 `Transport` 层的实现级规格文档。

它承接：

- [agent-society-p0-trust-discovery-transport-design.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-transport-design.md)
- [agent-society-p0-trust-discovery-policy-decisions.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-policy-decisions.md)
- [agent-social-runtime-api-and-roadmap.md](/home/ubuntu/fork_lk/linkclaw/docs/arch/agent-social-runtime-api-and-roadmap.md)

它的目标是回答：

1. `Transport` 层在 P0 里的固定 ownership 是什么
2. 哪些 route type 和 adapter 是 P0 范围内的
3. 发送、恢复、ack 的最小稳定接口是什么
4. 哪些运行态数据是 transport 的 source-of-truth

## 1. P0 的 `Transport` 边界

`Transport` 层只负责“如何搬运字节”和“如何反馈投递结果”。

它负责：

- route 对应的 adapter 执行
- `Send`
- `Sync`
- `Ack`
- adapter-normalized 的结果返回

它不负责：

- identity 可信性
- discovery source ranking
- route 候选排序
- 用户级 trust 判断

这些职责分别属于：

- `Trust`
- `Discovery`
- `Routing`
- `Runtime`

## 2. Package Ownership

## 2.1 `internal/transport`

P0 中 `internal/transport` 是 transport adapter 的权威 owner。

负责：

- route type 定义
- adapter interface
- adapter 实现
- adapter 测试

不负责：

- 生成 route candidates
- 计算 trust/freshness policy
- 生成用户可读状态文案

## 2.2 `internal/routing`

P0 中 `internal/routing` 负责 route planning，不属于 transport。

负责：

- direct / store-forward / recovery 的候选排序
- route outcome 记录

不负责：

- 实际网络执行

## 2.3 `internal/runtime`

P0 中 `internal/runtime` 负责编排 transport，不属于 transport 本身。

负责：

- 从 routing 拿 route candidates
- 找到支持该 route 的 adapter
- 聚合 send/sync/recover 结果
- 把 transport 结果转成产品语义

不负责：

- 实现 adapter 网络逻辑

## 3. P0 固定 route type

P0 中固定承认以下 route type：

- `direct`
- `store_forward`
- `recovery`

当前代码里虽然已经存在：

- `nostr`

但它不属于 P0 首批有效 route type。

因此：

- `RouteTypeNostr` 可以保留在代码里
- 但不进入 P0 首批交付和验收范围

## 4. P0 有效 adapter

P0 中有效 adapter 只有两类主能力和一个恢复形态：

### `direct`

首批实现形态：

- `libp2p_direct`

角色：

- 在线 peer 的优先直连通道

### `store_forward`

首批实现形态：

- runtime-owned store-forward adapter
- 当前兼容 legacy HTTP mailbox backend

角色：

- direct 失败时的离线投递 fallback

### `recovery`

角色：

- 面向离线投递恢复的 route 形态
- 本质上是恢复路径，而不是新的用户心智模型

## 5. 现有接口与 P0 固定接口

P0 直接建立在当前 `internal/transport.Transport` 接口上：

```go
type Transport interface {
    Name() string
    Supports(route RouteCandidate) bool
    Send(ctx context.Context, env Envelope, route RouteCandidate) (SendResult, error)
    Sync(ctx context.Context, route RouteCandidate) (SyncResult, error)
    Ack(ctx context.Context, route RouteCandidate, cursor string) error
}
```

P0 不新增第二套 adapter 接口。

原因：

- 当前接口已经覆盖 send/recover/ack 的最小需求
- 再拆第二套接口只会把 transport 层重新变复杂

## 6. 固定数据对象

## 6.1 `RouteCandidate`

P0 中固定字段：

- `Type`
- `Label`
- `Priority`
- `Target`

字段语义：

- `Type`
  - route 类别
- `Label`
  - 供 runtime/outcome/调试使用的稳定名
- `Priority`
  - routing 输出的候选顺序权重
- `Target`
  - adapter 可消费的目标描述

## 6.2 `Envelope`

P0 中 `Envelope` 保持最小：

- `MessageID`
- `SenderID`
- `RecipientID`
- `Plaintext`
- `Ciphertext`

P0 不把更多 trust/discovery 字段塞进 `Envelope`。

原因：

- 这些属于 routing/runtime policy 输入，不属于 transport payload

## 6.3 `SendResult`

P0 中字段保持：

- `Route`
- `RemoteID`
- `Delivered`
- `Retryable`
- `Description`

语义：

- `Delivered=true`
  - 表示 adapter 确认已经送达或已被可靠接收
- `Retryable=true`
  - 表示该失败或结果允许 runtime/routing 后续继续尝试

## 6.4 `SyncResult`

P0 中字段保持：

- `Route`
- `Recovered`
- `AdvancedCursor`

语义：

- `Recovered`
  - 本次 sync/recovery 恢复到本地的消息数
- `AdvancedCursor`
  - adapter 已推进的 mailbox cursor，若非空则 runtime 需要执行 ack

## 7. 运行态数据定位

P0 尽量复用当前 runtime 存储，不新建 transport 专属大表。

## 7.1 `runtime_route_attempts`

定位：

- transport outcome 的权威审计表

负责记录：

- route type
- route label
- success/failure
- retryable
- error
- cursor
- attempted_at

P0 中：

- 所有 send/sync/ack 关键结果都必须投影到这里或通过 routing outcome 进入这里

## 7.2 `runtime_messages.status`

定位：

- 用户可见消息状态的主字段

P0 中它不是 adapter 细节存储，而是产品读模型状态。

## 7.3 `runtime_presence_cache`

定位：

- route planning 和 transport readiness 的运行态输入

它不是 transport source-of-truth，但 transport 会消费它。

## 7.4 runtime store-forward state / cursor

定位：

- recovery 进度状态

P0 中 cursor 的权威前进由 transport `Sync` + `Ack` 决定。

## 8. 发送语义

P0 中发送必须遵循下面的编排语义：

1. runtime 获取 `PeerPresenceView`
2. routing 产出 `RouteCandidate[]`
3. runtime 依次选择 adapter
4. adapter 执行 `Send`
5. runtime 记录 outcome
6. 若 send 成功，返回产品级状态
7. 若 send 失败且 `Retryable=true`，继续尝试下一条 route
8. 若所有 route 均失败，返回明确错误

P0 要求：

- transport 层不自己决定下一条 route
- retry/fallback 顺序由 runtime + routing 驱动

## 9. 恢复语义

P0 中恢复必须遵循下面的编排语义：

1. runtime 获取 presence
2. routing 产出 recover routes
3. runtime 调用 adapter `Sync`
4. 若 `AdvancedCursor != \"\"`，runtime 调用 adapter `Ack`
5. runtime 记录 recovery outcome
6. read model 更新 inbox/thread

P0 要求：

- `Sync` 负责读 mailbox / pull pending deliveries
- `Ack` 负责推进确认位置
- 不能把 `Sync` 和 `Ack` 混成一个黑盒 side effect

## 10. 失败语义

P0 中 transport 失败分三类：

### Class A：可重试失败

例如：

- direct peer 不可达
- 临时网络错误
- store-forward 短暂服务异常

要求：

- `Retryable=true`
- runtime 继续尝试下一 route

### Class B：不可重试失败

例如：

- route target 无效
- payload 不合法
- adapter 不支持该 route

要求：

- `Retryable=false`
- runtime 可直接终止该 route

### Class C：恢复期失败

例如：

- sync 成功但 ack 失败
- cursor 推进不一致

要求：

- 结果必须被记录
- 不允许悄悄吞掉
- 后续 recovery 可再次尝试

## 11. `Transport` 层 DoD

P0 中 transport 层完成，至少要满足：

1. `direct`、`store_forward`、`recovery` 三类 route 在 runtime 下都可被编排
2. direct 失败后可稳定 fallback 到 store-forward
3. recovery 能返回 `Recovered` 数量并推进 cursor
4. 所有关键 outcome 都能进入 route attempt 审计
5. 用户面看不到原始 adapter 复杂性，但 debug/JSON 面可以看到

## 12. 与 Trust / Discovery 的接口关系

Transport 不消费完整的 trust profile，但它受 policy 影响。

具体关系如下：

- `Trust`
  - 决定是否允许高风险 direct
- `Discovery`
  - 决定 route hints 是否足够新鲜
- `Routing`
  - 根据 trust/discovery policy 排 route
- `Transport`
  - 执行已选 route

也就是说：

- trust/discovery 影响 transport
- 但 transport 不反向定义 trust/discovery

## 13. P0 不做的 Transport 事项

以下明确不进入 P0 transport 首批范围：

- Nostr 作为正式 transport
- 多 hop routing
- 群体广播
- always-on background delivery requirement
- 复杂 QoS / rate-limit policy

## 14. 结论

P0 的 `Transport` 层在实现上已经可以固定为：

- `internal/transport` 持有 adapter
- `internal/runtime` 编排 adapter
- `internal/routing` 决定 route 顺序
- `runtime_route_attempts` 持有关键审计结果
- `direct -> store_forward -> recovery` 构成 P0 的核心传输闭环

在这个边界下，Transport 已经具备进一步拆 workstream 的基础。 
