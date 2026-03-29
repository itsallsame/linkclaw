# Agent Society：Nostr 可恢复异步消息阶段计划

## 状态

这是 `Nostr 可恢复异步消息` 的阶段计划文档。

它承接：

- [agent-society-nostr-recoverable-async-messaging-design.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-nostr-recoverable-async-messaging-design.md)

这份文档的目标不是拆 ticket，而是先把这一轮开发划分成清晰阶段，并明确依赖关系。

## 为什么仍然先分阶段

虽然这轮最终会按你的要求放进一个 batch 一次性推进，但阶段划分仍然必要。

原因是：

- ticket 顺序要体现依赖关系
- 单 batch 也需要知道先做什么、后做什么
- 一旦中途出现 `blocked` 或设计偏差，PM 需要知道当前卡在哪一层

所以这里的“分阶段”不是为了拆多个 batch，而是为了给单 batch 排正确顺序。

## 阶段总览

这轮建议拆成 6 个阶段。

### Phase 1：Schema And Runtime Store

目标：

- 把 Nostr transport binding、relay、sync state、attempt、observation 的底座落库

范围：

- migration
- runtime store 扩展
- 基础 CRUD 与读取接口

退出标准：

- 新表可迁移
- store API 可读写
- 不改动现有 direct/store-forward 主路径语义

### Phase 2：Identity And Import Surfaces

目标：

- 让 self/peer 都能声明和导入 Nostr binding 与 relay hints

范围：

- `card export`
- `publish`
- `inspect`
- `import`

退出标准：

- self card 能导出 Nostr 字段
- inspect/import 能解析并落库
- peer binding 和 relay hints 能进入本地运行态存储

### Phase 3：Discovery And Runtime Aggregation

目标：

- 让 runtime 拿到 peer 的最终 relay/pubkey 视图

范围：

- discovery read model
- runtime read model
- relay/pubkey 优先级规则

退出标准：

- runtime 能得到 peer 的最终 transport binding 视图
- 手工覆盖、discovery、card、registry、默认 relay 的优先级规则成立

### Phase 4：Nostr Transport And Messaging Core

目标：

- 把 `direct-first + nostr-fallback + recover` 的主状态机做出来

范围：

- `transport/nostr`
- `message`
- `runtime bridge`
- sync/recover
- dedupe
- binding 校验

退出标准：

- 在线时 direct-first 成立
- direct 失败后至少一个 relay 成功即可进入 recoverable
- `sync` 可恢复消息
- 重复事件不会重复入库

### Phase 5：CLI / Plugin / Product Surface

目标：

- 把新能力接到用户入口，并保持产品语义一致

范围：

- CLI
- plugin
- `message status`
- debug/status surface

退出标准：

- CLI 能发送、sync、查看状态
- plugin 能做 background sync 和恢复提示
- 普通用户不需要理解 Nostr 细节

### Phase 6：Acceptance And Regression

目标：

- 对整条链路做回归、验收和证据沉淀

范围：

- unit / integration / contract / e2e
- 验收文档

退出标准：

- direct 成功
- recoverable async 成功
- relay 全失败状态正确
- partial relay success 状态正确
- binding 校验拒收成立

## 阶段依赖

依赖顺序建议为：

1. `Phase 1 -> Phase 2`
2. `Phase 2 -> Phase 3`
3. `Phase 3 -> Phase 4`
4. `Phase 4 -> Phase 5`
5. `Phase 5 -> Phase 6`

补充说明：

- `Phase 2` 的一部分实现会被 `Phase 4` 消费
- `Phase 3` 是 `Phase 4` 的直接输入
- `Phase 6` 不是可选尾声，而是本轮开发完成定义的一部分

## 单 Batch 推进方式

虽然这轮按单 batch 执行，但建议使用以下规则：

1. 单 batch 内 ticket 顺序必须严格体现上述阶段依赖
2. 当前依赖未完成前，不允许后序 ticket 进入真正实现
3. 若中间票 `blocked`，PM 应优先处理，不跳过依赖继续向后跑
4. 高冲突区域不并发修改

## 当前结论

这轮 Nostr 方案适合：

- 用一个 batch 一次性推进
- 但 ticket 顺序必须严格按阶段依赖组织

下一步应进入：

- [agent-society-nostr-recoverable-async-messaging-ticket-plan.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-nostr-recoverable-async-messaging-ticket-plan.md)
