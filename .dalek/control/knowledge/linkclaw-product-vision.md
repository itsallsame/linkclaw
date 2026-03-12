# LinkClaw 产品愿景与当前 User Story

> 创建日期：2026-03-12
> 状态：draft v0.1
> 依据：discussion.md

## 一、产品愿景

LinkClaw 要做的不是又一个去中心化身份协议，也不是一个 agent IM。

它的目标是为 OpenClaw 这类对外工作的 agent 提供一层可拥有、可迁移、可发现、可追责的个人互联网层，让 agent 不再只是挂在平台账号、聊天 channel 或单一框架里的“功能实例”，而是成为拥有自己名字、公开入口、身份工件和关系簿的互联网主体。

一句话定义：

**LinkClaw = 主权 agent 的开放身份、发现与关系层。**

## 二、我们到底在解决什么

当前 outward-facing agent 有三个结构性缺口：

1. 身份不属于 agent 主体自己，而是附着在平台账号、应用目录或厂商体系上。
2. 公开可发现的信息和私有信任关系混在一起，没有清晰的“公开名片 / 私有通讯录”边界。
3. 现有协议各自覆盖了身份、发现、传输、能力描述的一部分，但没有被收口成一个 OpenClaw 用户开箱就能拥有的产品层。

所以 LinkClaw 不试图重造这些底层协议，而是把它们压缩成一个极简但完整的默认组合：

- 一个可读名字：domain handle / public address
- 一个稳定身份：canonical identity
- 一张公开名片：可被抓取和解析的 profile/card
- 一本私有信任簿：contacts / trust / notes / policy
- 若干可插拔入口：A2A 只是第一种 adapter

## 三、产品定位

LinkClaw 的定位有几个明确边界：

- 它不是新的 identity primitive。
- 它不是新的公网传输协议。
- 它不是公共身份中心。
- 它不是“大而全”的 agent 网络。

它是一个 OpenClaw 原生的身份产品层，由三件东西组成：

1. 一个本地 identity core 插件。
2. 一个个人建站与发布 skill。
3. 一个公开索引层 LinkClaw。

## 四、长期价值判断

LinkClaw 长期值钱的，不是“消息怎么送达”，而是四件事：

1. 可拥有：agent 的身份、入口和关系不是平台资产。
2. 可迁移：入口可变，但主体和关系尽量不丢。
3. 可发现：别人可以通过普通 web 原语找到你。
4. 可追责：长期交互和默认授权能沉淀为关系历史。

进一步说，LinkClaw 不是在给 agent 加一个联系方式，而是在给 agent 加一套互联网原生人格的最小外壳：

- 域名和 handle：门牌号
- DID / keys：护照
- public profile / agent card：名片
- local trust book：关系簿

### 必要性判断

这里需要把一个容易混淆的判断写清楚：

- 对今天把 OpenClaw 主要当作个人多渠道助手使用的人来说，LinkClaw 不是普遍即时刚需。现有 channel identity、会话隔离、allowlist 和本地映射，已经能覆盖一部分日常使用。
- 但对 outward-facing、需要协作、委托、授权或交易的 agent 来说，某种身份、发现与信任层是结构性必要的。因为平台账号、sender id、A2A endpoint 或临时 URL 都只能回答“你现在从哪来”，不能稳定回答“长期你是谁”。

所以更准确的产品判断不是“所有 OpenClaw 用户现在都必须立刻拥有它”，而是：

**LinkClaw 是 OpenClaw 从个人助手走向对外主体、协作主体和交易主体时迟早要补的一层基础设施。**

这也决定了它的产品边界：

- LinkClaw 不去重造 transport。
- LinkClaw 不把 inbox address 当成 identity root。
- LinkClaw 先解决“主体是谁、如何被找到、为何被信任”，再考虑更复杂的社交和交易上层。

### 替代路径判断

围绕 OpenClaw，这里已经可以排除几条看起来相近、但不适合作为内核的路径：

- 只靠 email 或 IM address 不够，因为它们更像 transport endpoint，而不是长期 principal identity。
- 只靠 SNS profile 也不够，因为它更适合作为公开背书和发现表面，不适合作为源事实。
- 只靠 A2A 也不够，因为 A2A 更擅长描述“怎么调用这个 agent service”，不擅长回答“长期这是谁”。
- 直接上完整 DID/VC 体系，对当前 OpenClaw 阶段又偏重，成本高于收益。

因此更合适的方向不是再造一套大协议，而是把现成标准压缩成一个窄而硬的默认组合：

- `did:web` 或同类 domain-first identity 工件，负责长期 identity root
- `WebFinger`，负责发现入口
- `A2A Agent Card`，负责 adapter/export 视图
- local trust book，负责私有关系、备注、策略和风险判断

一句话说，LinkClaw 的内核不是邮箱模式，也不是 SNS 模式，而是：

**身份层为核心，邮箱 / IM / A2A / SNS 都是外围适配与信任表面。**

## 五、产品原则

1. 最古老的技术优先：优先 DNS、HTTPS、`/.well-known/`、已有标准工件。
2. 自托管优先：源事实优先留在用户自己的域名和本地数据里。
3. 本地优先：信任、备注、默认授权、交互历史默认不放到公网。
4. 组合优先：借用 DID、WebFinger、A2A Agent Card，而不是另造整套协议。
5. 低门槛优先：先让 OpenClaw 用户一装就能拥有，再谈大一统标准。

## 六、当前阶段的产品形态

当前讨论已经把 v0.1 的形态压缩到比较清晰：

### 1. OpenClaw 插件

作为本地 identity core 和 trust book。

它负责：

- 维护 self identity
- 管理 keys
- 维护 contacts / trust / notes / policy
- 导入和缓存他人的公开身份工件
- 对外导出 adapter 视图

### 2. 个人建站 skill

作为 publishing assistant。

它负责：

- 生成 human-readable profile
- 生成 machine-readable identity surface
- 指导用户把标准工件发布到自己的域名或静态托管上

### 3. LinkClaw 公共索引

作为 search/index 层。

它负责：

- 抓取公开工件
- 做归一化、搜索、标签和排序
- 指向源事实

它不负责：

- 充当身份权威
- 覆盖用户的源事实
- 取代用户自己的域名主页

## 七、当前用户角色与主 User Story

当前阶段的角色其实只有三类：

1. **agent owner**：运行 OpenClaw、拥有某只龙虾的人或组织。
2. **counterparty**：要与这只龙虾协作、连接、授权或交易的对端。
3. **ecosystem consumer**：读取公开身份信息的索引器、目录站点或协议适配器。

当前最核心的主 User Story 只有一个：

**作为一个拥有 OpenClaw agent 的个人或组织，我希望这只龙虾拥有独立于平台账号和单一协议的长期身份，这样它在跨 channel、跨入口、跨宿主工作时，仍然是一个可拥有、可迁移、可发现、可追责的互联网主体。**

## 八、当前重新梳理后的 User Stories

### Story 1：拥有长期身份

作为 agent owner，我希望我的龙虾有一个独立于聊天平台、目录站点和当前部署方式的长期身份，这样它换 channel、换宿主、换接入协议时，别人仍然认得它是谁。

### Story 2：平台账号不是身份本体

作为 agent owner，我希望别人看到的不只是“这只龙虾在某个平台里的账号”，而是一个独立于平台账号的长期主体，这样当它从 LinkedIn、Slack、A2A endpoint 或其他入口对外行动时，对端仍能判断这些入口是否属于同一只龙虾。

### Story 3：公开发布身份而不是公开全部关系

作为 agent owner，我希望能公开发布我的龙虾的身份名片和发现入口，同时把信任判断、备注和默认授权保留在本地，这样“别人如何找到我”和“我是否信任别人”是分层的。

### Story 4：验证对端并沉淀关系

作为 counterparty，我希望在协作、委托或交易之前能验证对方龙虾的身份，并把它纳入我的本地通讯录和信任记录，这样后续关系可以累积，而不是每次都重新判断。

### Story 5：让身份先于传输存在

作为 agent owner，我希望我的龙虾的身份不被某一种传输方式绑死，这样 IM、A2A、email、HTTP 或其他入口都只是适配层，而不是身份本体。

### Story 6：让生态能够消费公开身份而不成为权威

作为 ecosystem consumer，我希望能读取、抓取和索引龙虾公开发布的身份信息，这样生态里可以形成发现能力，但这些索引器和目录不会取代主体自己的源事实。

## 九、当前不追求的东西

当前阶段明确不追求：

- 新的 DID method
- 新的公网通信协议
- 全局声誉系统
- 全局身份中心
- 完整隐私层和匿名派生身份
- 区块链锚定

## 十、当前一句话产品结论

**LinkClaw 不是“OpenClaw 的一个 A2A 插件”，而是“给 OpenClaw agent 一键配上域名 handle、公开名片和本地信任簿的个人互联网层”。**
