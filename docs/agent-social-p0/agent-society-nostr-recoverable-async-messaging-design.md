# Agent Society：Nostr 可恢复异步消息设计

## 状态

这是面向下一轮开发规划的设计文档。

它建立在以下现有文档之上：

- [agent-society-value-notes.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-society-value-notes.md)
- [agent-society-p0-trust-discovery-transport-design.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-trust-discovery-transport-design.md)
- [agent-society-p0-development-summary.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-development-summary.md)

这份文档不讨论 `Collaboration` 或 `Value`。

它只回答一个问题：

- 如何在现有 `Trust / Discovery / Transport` 基线之上，引入 `Nostr` 作为第三方 relay 方案，完成 `direct-first + 可恢复异步消息`

## 1. 开发目标 / 非目标 / 边界

### 1.1 开发目标

本轮开发目标明确为：

1. 保持 `direct-first`
2. 在对方离线时，通过第三方 `Nostr relay` 提供可恢复的异步消息路径
3. 保持 `LinkClaw canonical identity` 作为主身份，不让 `Nostr` 取代身份本体
4. 用产品语义暴露能力，而不是让普通用户直接理解 transport 细节

更准确地说，这轮目标不是“能把消息异步发出去”，而是：

- 发送方离线时可以先发
- 消息进入可恢复状态
- 接收方后续可以通过 `sync` 取回
- 同步过程在多 relay、多 pubkey 条件下仍然可去重、可校验、可观察

### 1.2 非目标

这轮明确不做：

- 群聊
- 公开 feed 或社交事件流
- `Collaboration Layer`
- reputation / market
- 文件离线恢复
- 自建 relay server
- 本地自动拉起 relay 进程
- 强匿名通信
- 完整联邦和多宿主共识协议

### 1.3 边界

这轮边界明确为：

- 使用第三方 `Nostr relay`
- 仍然默认 `direct-first`
- `Nostr` 只承担私有一对一消息的离线恢复路径
- 第一版允许多个 `nostr_pubkey`
- 第一版允许多个 relay
- 客户端内置默认公共 relay 列表
- hand-import 的 peer 允许手工补 relay

## 2. 身份绑定与 Relay 配置模型

### 2.1 主身份与 Transport 身份分层

`LinkClaw canonical identity` 仍然是主身份。

`Nostr pubkey` 被视为 transport binding，而不是 identity replacement。

也就是说，关系应表达为：

- `canonical_id -> nostr_pubkey[] -> relay_urls[]`

而不是把 `relay` 或 `nostr_pubkey` 直接当成主身份。

### 2.2 绑定模型

建议引入概念对象 `TransportBinding`：

- `binding_id`
- `owner_kind`
- `owner_id`
- `canonical_id`
- `transport_type`
- `public_key`
- `is_primary`
- `source`
- `verified_state`
- `created_at`
- `updated_at`

对本轮来说：

- `transport_type = nostr`
- 一个 identity 可绑定多个 `nostr_pubkey`

### 2.3 Relay 配置模型

建议引入概念对象 `RelayEndpoint`：

- `relay_id`
- `owner_kind`
- `owner_id`
- `canonical_id`
- `transport_type`
- `relay_url`
- `scope`
- `priority`
- `enabled`
- `source`
- `created_at`
- `updated_at`

`scope` 第一版建议支持：

- `declared`
- `manual`
- `default`

### 2.4 公开声明与运行态缓存分层

这一轮应同时存在两层：

1. 公开声明层

- 写入 `agent-card`
- 让 peer 能看见：
  - `nostr_public_keys[]`
  - `nostr_primary_public_key`
  - `relay_urls[]`
  - `transport_capabilities` 包含 `nostr`

2. 本地运行态缓存层

- 写入 runtime/discovery store
- 让 send/sync/recover 可以直接消费

### 2.5 Relay 来源优先级

relay 选择优先级建议为：

1. 本地手工覆盖
2. peer 最新 discovery/runtime record
3. peer identity card / publish surface
4. registry record
5. 客户端内置默认公共 relay

### 2.6 多 pubkey 优先级策略

多 pubkey 优先级建议为：

1. 本地手工指定的 pubkey
2. 最近成功收发过的 pubkey
3. 在 peer identity/card 中标记为 primary 的 pubkey
4. 其余已声明 pubkey，按最近更新时间排序

### 2.7 第一版绑定校验规则

第一版采用“弱验证绑定”：

- sender pubkey 必须能映射到某个已知 peer 的声明 pubkey
- receiver target pubkey 必须属于自己的已绑定 pubkey 集合

这意味着第一版必须支持双边校验：

- `sender pubkey -> known peer canonical_id`
- `receiver target pubkey -> self canonical_id`

## 3. 发送 / Sync / Recover 状态机

### 3.1 用户可见状态

建议对用户暴露这些状态：

- `pending`
- `direct_delivered`
- `deferred_recoverable`
- `recovered`
- `failed_unrecoverable`

### 3.2 内部状态

系统内部可分解为：

- `created`
- `direct_attempting`
- `direct_failed`
- `relay_queueing`
- `relay_queued`
- `relay_partial`
- `relay_failed`
- `recover_pending`
- `recovered`
- `terminal_failed`

### 3.3 发送流程

发送流程应为：

1. 本地创建消息
2. 尝试 direct
3. direct 成功则结束，状态为 `direct_delivered`
4. direct 失败则进入 relay 目标选择
5. 并行写入多个 relay
6. 至少一个 relay 成功，则状态为 `deferred_recoverable`
7. 全部 relay 失败，则状态为 `failed_unrecoverable`

### 3.4 并行写入策略

第一版建议：

- relay 写入并行执行
- 至少一个 relay 成功即可判定为 recoverable
- 不要求全部 relay 成功

这与“多个 relay 作为冗余恢复路径”的目标一致。

### 3.5 Sync 位点策略

第一版位点建议按以下组合存储：

- `last_seen_created_at`
- `last_seen_event_id`

不要只用时间戳，也不要只用 event id。

第一版应按：

- `relay_url x receiver_public_key`

分别推进同步位点。

### 3.6 Dedupe 策略

建议：

- 业务消息主键以 `message_id` 为准
- transport 观测主键以 `(event_id, relay_url, receiver_public_key)` 为准

结果是：

- 同一条业务消息只入库一次
- 同一消息在多个 relay 上可出现多条 observation

### 3.7 Ack 语义

第一版 ack 建议定义为：

- `client-side ack`

也就是：

- 本地标记已消费
- 不要求第三方 relay 提供真正的 mailbox delete 语义

### 3.8 最小恢复门槛

第一版 recoverable 的判定为：

- 至少 `1` 个 relay 成功写入即可

不要求最少 `2` 个成功。

## 4. 验收标准与测试设计

### 4.1 产品级验收标准

本轮至少需要通过以下 8 类验收。

1. `Direct-first`

- 对方在线时优先走 direct
- 不误入 relay 恢复路径

2. `Deferred recoverable`

- 对方离线时，至少一个 relay 成功即可进入 recoverable 状态

3. `Unrecoverable failure`

- direct 全失败且 relay 全失败时，状态必须为最终失败

4. `Recovery sync`

- 接收方后续 `sync` 能把消息恢复进本地线程

5. `Dedupe`

- 同一消息被多个 relay 返回时，只入库一次

6. `Binding validation`

- 未通过绑定校验的事件不能进入正式消息流

7. `Partial relay success`

- 部分 relay 成功时，整体仍视为 recoverable

8. `Third-party relay instability`

- relay 超时、限流、报错时，系统行为仍然可观察、可诊断

### 4.2 测试分层

建议测试分为 4 层：

1. Unit tests

- 位点推进
- dedupe
- relay 选择
- pubkey 优先级
- 状态转换
- 绑定校验

2. Integration tests

- direct success
- direct fail -> relay queued
- relay sync -> recovered
- all relay fail -> failed
- partial relay success

3. Contract tests

- 与第三方 relay 的 publish/pull 假设兼容
- 过滤与排序语义
- 异常响应处理

4. E2E acceptance tests

- `discover -> connect -> send -> direct delivered`
- `discover -> connect -> send while offline -> queued for recovery`
- `recipient sync -> recovered`
- `duplicate relay events -> one message only`
- `invalid binding event -> rejected`

### 4.3 总完成定义

这轮总 DoD 建议为：

1. 在线时 direct-first 成立
2. 离线时至少一个 relay 成功即可 recoverable
3. `sync` 能恢复消息
4. 多 relay / 多 pubkey 下不重复入库
5. binding 校验能阻止伪造事件进入正式消息流
6. relay 异常时状态可观察
7. CLI / runtime / plugin 的产品语义一致
8. 自动化回归与 e2e 验收证据齐备

## 5. 数据模型与存储设计

### 5.1 设计原则

设计原则为：

1. `canonical_id` 仍然是主身份锚点
2. `Nostr` 只是 transport binding
3. 区分声明数据与运行态数据
4. 去重与恢复状态必须落库

### 5.2 建议新增对象

建议新增 5 类概念对象：

1. `TransportBinding`

- 支持多个 `nostr_pubkey`

2. `RelayEndpoint`

- 支持多个 relay 与默认 relay

3. `RelaySyncState`

- 按 `relay x receiver_public_key` 记录同步位点

4. `RelayDeliveryAttempt`

- 记录一条消息向多个 relay/pubkey 的投递结果

5. `RecoveredEventObservation`

- 记录从哪个 relay 看到了哪个 event，以及是否被接受

### 5.3 建议新增表

建议新增运行态表：

- `runtime_transport_bindings`
- `runtime_transport_relays`
- `runtime_relay_sync_state`
- `runtime_relay_delivery_attempts`
- `runtime_recovered_event_observations`

### 5.4 与现有对象的关系

继续保留现有主对象：

- `self_identities`
- `contacts`
- `runtime_contacts`
- `messages`
- `route_attempts`
- `discovery_records`

新的 Nostr 数据不直接塞进这些主表，而以独立运行态表承载。

### 5.5 Agent Card 扩展

这一轮 `agent-card` 应能公开声明：

- `nostr_public_keys[]`
- `nostr_primary_public_key`
- `relay_urls[]`
- `transport_capabilities` 包含 `nostr`

### 5.6 默认公共 Relay 的落地方式

默认公共 relay 建议：

- 在客户端内置常量列表
- 不直接预写数据库
- 只有当运行时真正使用默认 relay 时，才以 `scope=default` 落到本地记录

## 6. 模块边界与 Workstream 划分

### 6.1 模块边界

这一轮建议分成 6 个模块面：

1. `Identity / Card / Publish`

- 负责公开声明 Nostr binding 与 relay

2. `Discovery / Runtime Read Model`

- 负责聚合 peer 的 relay/pubkey 视图

3. `Transport / Messaging`

- 负责 direct-first、relay fallback、sync/recover、dedupe、状态推进

4. `Storage / Migration`

- 负责新表与运行态持久化

5. `CLI / Product Surface`

- 负责 send/sync/status/plugin 的产品语义输出

6. `Testing / Acceptance`

- 负责 unit / integration / contract / e2e 封口

### 6.2 Workstream 划分

建议拆成 6 个 workstream：

1. `A Schema 与运行态存储`
2. `B Identity/Card/Import 扩展`
3. `C Discovery/Runtime 聚合`
4. `D Nostr Transport + Messaging 状态机`
5. `E CLI / Plugin / Status`
6. `F Acceptance / Regression`

### 6.3 推荐执行顺序

推荐顺序：

1. `A`
2. `B`
3. `C`
4. `D`
5. `E`
6. `F`

### 6.4 并行建议

可以有限并行：

- `B` 与 `C`
- `E` 在 `D` 后半段提前介入
- `F` 在 `D/E` 阶段同步补测试

不建议并行：

- `A` 与 `D`
- 多个 worker 同时修改：
  - `internal/message/service.go`
  - `internal/message/runtime_bridge.go`
  - `internal/runtime/store.go`
  - `internal/runtime/service.go`
  - `internal/cli/run.go`

### 6.5 各 Workstream 完成定义

`A`

- migration 可跑
- 新表存在
- store API 可读写

`B`

- self card 能导出 Nostr binding/relay
- inspect/import 能解析并落库

`C`

- runtime 能拿到 peer 的最终 relay/pubkey 视图
- 优先级规则已实现

`D`

- direct-first 成立
- direct 失败可写 relay
- 至少一个 relay 成功即 recoverable
- `sync` 可恢复消息
- dedupe / binding 校验成立

`E`

- CLI / plugin / status 的产品语义一致

`F`

- 自动化回归齐全
- e2e 验收完成
- 验收证据沉淀

## 7. 当前结论

到这一步，围绕 `Nostr` 的可恢复异步消息设计，已经收敛了：

1. 开发目标 / 非目标 / 边界
2. 身份绑定与 relay 配置模型
3. 发送 / sync / recover 状态机
4. 验收标准与测试设计
5. 数据模型与存储设计
6. 模块边界与 workstream 划分

这份文档已经足够进入正式的阶段计划和 ticket 拆分。

下一步应进入：

- phase plan
- ticket plan
- 依赖关系与 batch 编排
