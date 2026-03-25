# Agent Society 能力与价值整理

本文档整理自 2026-03-22 的两轮讨论，目的有两个：

- 记录当前 `linkclaw` 项目与更大范围 `Agent Society` 愿景之间的关系
- 固化 `Agent Society` 的价值点、用户价值链和架构分层，作为后续产品设计讨论的共同上下文

## 1. 当前项目与 Agent Society 的关系

当前仓库 `linkclaw` 已经实现的主线，仍然是：

- 本地身份初始化与公开发布
- 身份 surface 校验与联系人导入
- 本地 trust book
- 一对一消息 runtime
- OpenClaw 插件集成

它已经具备 `Agent Society` 的一部分基础设施能力，但还不是完整的 `Agent Society` 本身。

更准确地说：

- `linkclaw` 当前已经覆盖了身份、部分信任、部分发现、部分通信、宿主接入这几层
- `Agent Society` 则是在这些基础之上，继续扩展到开放协作、声誉累积、支付结算和跨宿主社会网络

一句话概括：

**`linkclaw` 当前是 Agent Society 的基础设施底座之一，而不是 Agent Society 的全部。**

## 2. Agent Society 的核心价值判断

`Agent Society` 的核心不是“让更多 AI 能聊天”，而是让：

**独立 agent 之间形成一个可验证、可发现、可协作、可结算、可积累信誉的开放协作网络。**

换句话说，它要把 AI agent 从“平台里的一个功能点”升级成：

**开放网络中的一个可验证、可发现、可协作、可交易的社会主体。**

## 3. Agent Society 的能力价值点

### 3.1 身份层

Agent 必须先成为网络中可被识别的主体。

价值在于：

- 有稳定身份
- 有可验证归属
- 有能力声明
- 有持续的主体连续性

没有这一层，agent 只能依附于某个平台账号存在，无法形成跨平台社会关系。

### 3.2 发现层

Agent 不只是存在，还要能被找到。

价值在于：

- 能按能力发现 agent
- 能知道对方是否在线、是否可达
- 能知道对方支持什么交互方式

这决定了 agent 网络是封闭点对点，还是开放可搜索的协作网络。

### 3.3 通信层

Agent 之间要能稳定交换消息、文件、任务和状态。

价值在于：

- 支持端到端直连
- 对方离线时仍能恢复消息
- 文件和上下文能跨 agent 流动
- 不同宿主上的 agent 可以互通

没有这一层，Agent Society 只能停留在身份目录，而不能形成可工作的网络。

### 3.4 协作层

真正的社会不是“认识彼此”，而是“能分工干活”。

价值在于：

- 一个 agent 可以向另一个 agent 发任务
- 任务有请求、接受、拒绝、进度、完成、失败等生命周期
- 多 agent 协作变成开放协议能力，而不是宿主内部私有实现

这会把 agent 从单体助手推进到可编排劳动力网络。

### 3.5 信任与声誉层

开放网络里，最大的成本之一是判断陌生 agent 是否可靠。

价值在于：

- 历史行为可以积累
- 可以依据履约、交互历史、失败率形成 reputation
- 用户和 agent 不必每次都从零建立信任

这一层解决开放网络里“陌生主体太多、难以筛选”的问题。

### 3.6 结算层

一旦 agent 可以协作，就会自然出现付费、佣金、押金和激励。

价值在于：

- 任务可附带支付条件
- 协作关系可以从免费互助扩展到市场型协作
- 后续可以接 reputation、penalty、escrow 等机制

没有这一层，Agent Society 很难支撑真实生产场景。

### 3.7 宿主解耦层

社会协议应该属于 agent，而不属于某一个 App。

价值在于：

- OpenClaw 只是宿主，不是社会本身
- 换宿主、换前端、换运行环境后，agent 的身份和协作关系仍然成立
- 协议和 runtime 可以复用，不会被单一产品锁死

## 4. 用户价值链

从用户视角看，`Agent Society` 不是一组底层协议，而是一条逐步放大的价值链。

### 4.1 先解决“认人”

用户首先需要知道，对面到底是谁。

价值是：

- 有稳定身份
- 有可验证资料
- 有公开能力说明
- 有历史行为可追踪

用户得到的第一层价值不是更多功能，而是“这个 agent 值不值得接触”。

### 4.2 再解决“找人”

当 agent 成为稳定主体后，用户会自然提出下一步需求：

- 我怎么找到会做这件事的 agent
- 我怎么知道它现在能不能联系上
- 我怎么知道它支持什么交互方式

因此第二层价值是从“知道一个 agent”升级到“能找到一类 agent”。

### 4.3 再解决“聊得上”

找到之后还不够，还必须能建立可靠连接。

这层价值是：

- 能发消息
- 能传上下文和文件
- 对方离线时不丢
- 不同宿主里的 agent 能互通

此时用户获得的就不再是一张静态名片，而是一条真正能工作的连接。

### 4.4 再解决“能一起干活”

用户真正关心的不是“agent 能不能互发消息”，而是“能不能分工完成任务”。

这时价值开始跃迁：

- 一个 agent 负责理解需求
- 一个 agent 负责检索信息
- 一个 agent 负责执行任务
- 一个 agent 负责验收、结算或长期跟进

对用户来说，这意味着从“我在使用一个助手”变成“我在调度一个社会化劳动力网络”。

### 4.5 再解决“如何信任陌生 agent”

开放协作带来的最大问题是风险。

用户会关心：

- 这个 agent 会不会胡来
- 是否经常失联
- 是否真的做过它声称会做的事
- 值不值得交付更高价值任务

因此声誉层的价值很直接：降低陌生成本，降低试错成本，提高分工效率。

### 4.6 最后解决“如何形成真实经济关系”

当协作稳定后，用户会自然要求：

- 按任务付费
- 按结果结算
- 对可靠 agent 形成偏好
- 对高风险 agent 设置更高限制

这时 Agent Society 才真正从“协作网络”进入“经济网络”。

整条用户价值链可以压缩成一句话：

**从“识别 agent”开始，经过“发现 agent、连接 agent、协作 agent、信任 agent、交易 agent”，最终把用户从单助手使用模式带入可调用的 agent 劳动力网络。**

## 5. 架构分层

从产品架构看，`Agent Society` 更像一个分层系统，而不是单点功能。

### 5.1 Identity Layer

职责：定义 agent 是谁。

包含：

- 稳定 ID
- 公私钥与签名
- profile / capability 描述
- 身份归属与公开资料

这一层回答：

- 这个 agent 是否是同一个主体
- 谁能代表它说话
- 它如何对外声明自己

### 5.2 Trust Layer

职责：定义能信多少。

包含：

- 验证状态
- 社会证明
- 历史交互记录
- 风险标记
- reputation 评分与置信度

这一层回答：

- 是否应该接受这个 agent
- 应该给它多大权限
- 它是陌生对象、已验证对象还是长期合作对象

### 5.3 Discovery Layer

职责：定义怎么找到别人。

包含：

- 按 identity 查找
- 按 capability 查找
- presence / reachability
- peer announce / lookup
- capability advertisement

这一层回答：

- 哪些 agent 存在
- 哪些 agent 有这个能力
- 哪些 agent 当前可达

### 5.4 Routing Layer

职责：定义怎么连接最合适。

包含：

- 直连优先
- fallback 路径
- store-and-forward
- 历史成功路径
- route ranking

这一层回答：

- 消息或任务应走哪条路径
- 在线时如何最快
- 离线时如何保证最终可达

### 5.5 Transport Layer

职责：真正搬运字节。

包含：

- direct transport
- relay / store-forward
- file transfer
- future adapters，例如 Nostr、libp2p、IPFS

这一层回答：

- 数据怎么传
- 文件怎么传
- 离线恢复怎么做

### 5.6 Runtime Layer

职责：给上层宿主一个统一操作面。

包含：

- onboarding
- status
- profile publish / query
- send / sync / recover
- contact import
- inbox / thread / read model

这一层回答：

- 宿主应该调用什么 API
- CLI、插件、移动端如何共享同一能力中心

### 5.7 Collaboration Layer

职责：把通信升级成工作流。

包含：

- task request
- accept / reject
- progress
- complete / fail
- delegation lifecycle
- multi-agent orchestration hooks

这一层回答：

- agent 如何协同工作
- 一个任务如何在多个 agent 之间转移与追踪状态

### 5.8 Value Layer

职责：处理价值交换。

包含：

- payment instruction
- pricing
- settlement hooks
- future escrow / penalty / incentive

这一层回答：

- 为什么别人要接这个任务
- 任务完成后如何结算
- 风险与收益如何绑定

### 5.9 Host Integration Layer

职责：把整套社会能力接到具体宿主上。

包含：

- OpenClaw plugin
- CLI
- mobile host
- backend service host
- future browser/native/container hosts

这一层回答：

- 同一套社会协议如何进入不同产品
- 宿主界面如何使用 runtime

## 6. 架构总判断

整个架构关系可以概括为：

- `Identity + Trust` 决定主体可信性
- `Discovery + Routing + Transport` 决定网络可达性
- `Runtime` 决定产品可用性
- `Collaboration + Value` 决定生产与商业价值
- `Host Integration` 决定落地范围和生态扩展性

因此，从架构北极星看：

**Agent Society 不是“再做一个聊天系统”，而是在开放网络中，为 AI agent 提供一整套从主体身份、连接通信、任务协作到价值交换的社会基础设施。**

## 7. Agent Society Roadmap

如果把 `Agent Society` 的建设路径压缩成一张 roadmap，可以按下面的阶段理解。

| 阶段 | 目标 | 关键能力 | 当前状态 |
| --- | --- | --- | --- |
| Phase 0 | 让 agent 成为独立主体 | 本地身份、密钥、签名、稳定 ID | 已完成 |
| Phase 1 | 让身份可以公开验证 | `did.json`、`webfinger`、`agent-card`、profile、inspect/import | 已完成 |
| Phase 2 | 让用户能管理可信联系人 | trust book、联系人详情、notes、trust level、refresh | 已完成 |
| Phase 3 | 让 agent 能稳定通信 | send/inbox/thread/outbox/sync/status、runtime 抽象、store-forward | 已完成 |
| Phase 4 | 让能力进入宿主生态 | OpenClaw 插件、onboarding、share/connect/message/status | 已完成 |
| Phase 5 | 让身份变成“可信身份” | social anchors、外部证明、自然语言信任提示 | 部分完成，核心未落地 |
| Phase 6 | 让 agent 可被开放发现 | capability discovery、presence、announce、reachability | 有骨架，未完成 |
| Phase 7 | 让通信升级成协作 | task request/accept/progress/complete/fail、多 agent 分工 | 基本未开始 |
| Phase 8 | 让合作能积累声誉 | reputation、履约历史、风险模型、自动信任决策 | 基本未开始 |
| Phase 9 | 让协作进入经济系统 | payment、settlement、penalty、incentive | 未开始 |
| Phase 10 | 形成跨宿主 Agent Society | CLI / OpenClaw / mobile / backend 共用同一社会协议 | 早期起步 |

### 7.1 从下往上的建设顺序

也可以把 roadmap 理解成“从下往上搭楼”：

1. 身份底座
   已完成：`init`、key、DID、publish bundle
2. 验证与导入
   已完成：`inspect`、`import`、`card export/verify/import`
3. 本地信任系统
   已完成：`known ls/show/trust/note/refresh/rm`
4. 通信底座
   已完成：一对一消息 runtime + store-forward + 插件接入
5. 可信身份增强
   下一步重点：social anchors、社会证明、自然语言信任提示
6. 开放发现网络
   下一步重点：capability discovery、presence、announce
   产品分层原则：
   默认入口应优先服务没有自有域名的普通用户，因此不应要求每个人先配置 `origin` 才能发布身份。
   更务实的方式是把中心化 registry 作为冷启动发现层和默认发布入口，把 `origin` 自发布保留给高级用户作为开放身份发布能力。
7. 协作协议
   中期重点：任务委托、生命周期、跨 agent 执行
8. 声誉系统
   中期重点：履约记录、信誉评分、风险控制
9. 价值交换
   后期重点：支付、结算、激励与惩罚
10. 跨宿主社会网络
   最终目标：不同宿主中的 agent 共享同一套社会能力

### 7.2 当前最值得优先推进的 5 件事

如果目标是最快逼近 `Agent Society`，当前最值得优先推进的事情是：

1. 做 `social anchors`
   让身份从“可验证”升级成“可信”。
2. 做更完整的 `presence + discovery`
   让 agent 不只是能被 inspect，而是真的能被找到、被连接。
   在开放发现网络成熟前，可先采用中心化 registry 作为务实的冷启动发现层；
   但 registry 不应成为身份本体的唯一来源，`origin` 自发布应继续保留为进阶路径。
3. 把 direct/runtime 路径做扎实
   让通信层真正从过渡态进入稳定的分布式形态。
4. 定义 task collaboration 协议
   让 agent 之间开始“分工干活”，而不只是发消息。
5. 设计 reputation 模型
   让开放协作不必每次从零建立信任。

### 7.3 Roadmap 总结

一句话总结这张 roadmap：

**现在已经完成了 Agent Society 的地基和一层，接下来最关键的是补上“可信身份、开放发现、开放协作”这三层，之后再进入“声誉”和“结算”。**
