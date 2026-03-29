# Agent Society P0：Transport 开发工作流与验收边界

## 状态

这是 `Agent Society P0` 面向 `Transport` 层的开发边界文档。

它承接：

- [agent-society-p0-transport-implementation-spec.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-transport-implementation-spec.md)
- [agent-society-p0-transport-policy-decisions.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-p0/agent-society-p0-transport-policy-decisions.md)

它的目标是把 Transport 层切成可开发的 workstream，并定义：

- ownership
- 串并行依赖
- 验收边界

## 1. Workstream 总览

P0 的 Transport 建议拆成四个 workstream。

### TWS1：Adapter And Route Type Stabilization

目标：

- 固定 P0 内的 route type 和 adapter 边界

核心内容：

- `internal/transport/types.go`
- `internal/transport/libp2p/*`
- `internal/transport/storeforward/*`

### TWS2：Runtime Transport Orchestration

目标：

- 把 runtime 的 send/sync/recover 编排做稳定

核心内容：

- `internal/runtime/service.go`
- hooks / outcome record
- adapter 选择与 fallback

### TWS3：Transport Outcome And Status Surface

目标：

- 把 route outcome、recovery、transport-ready 状态稳定暴露出来

核心内容：

- `runtime_route_attempts`
- runtime status surface
- message status / recovery status

### TWS4：CLI / Plugin User Surface

目标：

- 把 transport 结果转成稳定产品语义

核心内容：

- CLI `message send/sync/status`
- plugin 侧状态和结果 copy

## 2. Ownership

## 2.1 TWS1

拥有文件：

- `internal/transport/types.go`
- `internal/transport/libp2p/*`
- `internal/transport/storeforward/*`
- 相关 transport 测试

## 2.2 TWS2

拥有文件：

- `internal/runtime/service.go`
- `internal/runtime/types.go`
- `internal/runtime/hooks.go`
- `internal/runtime/*_test.go`

## 2.3 TWS3

拥有文件：

- runtime store / status 汇总
- route outcome 读模型
- message status surface

## 2.4 TWS4

拥有文件：

- `internal/cli/run.go`
- `internal/cli/*_test.go`
- `openclaw-plugin/src/*`
- `openclaw-plugin/test/*`

## 3. 依赖关系

必须串行：

1. `TWS1 -> TWS2`
2. `TWS2 -> TWS3`
3. `TWS2 + TWS3 -> TWS4`

可以并行：

- `TWS1` 内 direct/store-forward 子实现
- `TWS4` 内 CLI 与 plugin 适配

## 4. 完成标准

## 4.1 TWS1 DoD

- `direct` / `store_forward` / `recovery` 三类 route 行为稳定
- transport tests 覆盖支持/不支持路径
- `nostr` 不进入 P0 首批行为

## 4.2 TWS2 DoD

- runtime send 能 direct-first
- direct 失败能 fallback 到 store-forward
- recovery 能触发 sync + ack
- outcome 全部记录

## 4.3 TWS3 DoD

- transport-ready 状态可读
- message status 能反映 direct / deferred / recovered
- recovery 结果对 CLI / plugin 可见

## 4.4 TWS4 DoD

- CLI 普通输出使用产品语义
- plugin 不泄露底层 adapter 细节
- E2E 覆盖在线发送、离线恢复两条路径

## 5. 验收顺序

建议按以下顺序验收：

1. adapter 单测
2. runtime orchestration 单测
3. status/read-model 测试
4. CLI / plugin 路径测试
5. E2E 在线发送
6. E2E 离线恢复

## 6. 结论

到这一步，`Transport` 层也已经具备：

- 实现级规格
- 策略决策
- workstream 和验收边界

因此，P0 的三个层级现在在文档深度上已经基本对齐。 
