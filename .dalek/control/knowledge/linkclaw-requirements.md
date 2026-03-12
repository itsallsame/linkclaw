# LinkClaw 需求文档

> 创建日期：2026-03-12
> 状态：draft v0.1
> 依据：discussion.md

## 一、文档目的

本文档定义 LinkClaw v0.1 的产品边界、核心需求和验收标准。

本阶段目标不是发明新的 identity protocol，而是把现有 web-native 原语和标准工件收束成一个 OpenClaw 用户真正能拥有、部署和使用的身份层产品。

## 二、产品定义

LinkClaw 是一个面向 OpenClaw agent 的 identity core + discovery + trust 产品层。

v0.1 由三部分组成：

1. OpenClaw 本地插件：identity core + private trust book
2. 个人建站 skill：public identity surface 发布助手
3. LinkClaw 公共索引：公开工件的搜索与归一化索引

## 三、v0.1 目标

v0.1 需要满足以下目标：

1. 让一个 OpenClaw 用户可以为自己的 outward-facing agent 建立稳定身份根。
2. 让该身份根独立于单一平台账号、单一 channel id 或单一 service endpoint。
3. 让该用户可以把公开身份工件发布到自己的域名或静态站点上。
4. 让另一个用户或 agent 可以根据 domain / URL / profile 发现并导入这些公开工件。
5. 让本地系统能够把公开身份转化成私有联系人和信任关系。
6. 让身份核心可以导出 A2A Agent Card 这类 adapter 视图，但不以它们作为 source of truth。

### 当前服务的角色

v0.1 当前服务三类角色：

1. **agent owner**：拥有并运行 OpenClaw agent 的个人或组织。
2. **counterparty**：需要识别、验证、连接或与该 agent 交易的对端。
3. **ecosystem consumer**：读取公开身份工件的索引器、目录层和协议适配器。

### 当前关键场景

v0.1 的需求应始终围绕以下场景展开：

1. **我拥有自己**：owner 为自己的龙虾建立长期身份。
2. **平台入口不是我本体**：同一只龙虾在不同平台、账号和 endpoint 上出现时，对端仍能判断这些入口是否属于同一长期主体。
3. **别人找到我**：owner 将公开身份工件发布到普通 web 环境。
4. **我识别别人**：counterparty 从公开入口拉取身份信息并导入本地信任簿。
5. **关系能沉淀**：导入后的身份可以在本地持续积累备注、策略和交互历史。
6. **传输不是身份本体**：A2A 等协议只消费 identity core 的导出视图，而不成为 source of truth。

### 必要性判断

这里需要明确写下产品边界，避免把 v0.1 误写成“所有 OpenClaw 用户都立刻需要的通用层”。

- 对主要把 OpenClaw 当作个人多渠道助手使用的用户，这套能力不是当前普遍即时刚需。
- 对 outward-facing、需要长期协作、授权或交易的 agent，这套能力是结构性必要。

因此 v0.1 不是为了替代 OpenClaw 现有 gateway、session 和 channel identity 机制，而是为了补上它们尚未提供的长期 identity、公开发现和私有 trust 沉淀能力。

这里还要补一个关键判断：

- 平台账号、channel user id、service endpoint 可以作为入口标识。
- 但它们不能单独承担 principal identity，因为它们默认回答的是“你在这里是谁”，不是“长期你是谁”。

### 替代方案判断

v0.1 需求在设计上排除以下几种看似相近、但不应作为内核的方案：

1. **邮箱模式作为内核**：email address 或 inbox address 可以作为 adapter，但不应成为 identity root。
2. **SNS 模式作为内核**：LinkedIn、X、GitHub 等 profile 适合作为 social proof 和发现表面，不应成为唯一源事实。
3. **A2A-only**：A2A Agent Card 解决的是 service description 和调用入口，不足以承载长期 principal identity。
4. **完整 DID/VC 先行**：能力强，但对 OpenClaw 当前阶段过重，不适合做 v0.1 默认路径。

因此 v0.1 的推荐组合应明确为：

1. `domain + key material + local trust book` 作为 source of truth
2. `did.json + WebFinger + agent-card.json + human-readable profile` 作为 public identity surface
3. `A2A / IM / email / HTTP` 作为 transport or adapter

如果后续需要消息级验证，v0.1 只要求为高价值动作预留 signed envelope 能力，而不要求为所有闲聊消息强制附带消息级签名。

## 四、核心概念

### 1. handle

人类可读的公开入口，通常表现为域名、URL 或可解析的公开地址。

### 2. canonical identity

相对稳定的主体标识，用于表示“长期你是谁”，而不是“此刻从哪个入口访问你”。

### 3. public identity surface

发布在公开 web 上、可被抓取和解析的身份工件集合，例如：

- `did.json`
- `/.well-known/webfinger`
- `/.well-known/agent-card.json`
- human-readable profile page

### 4. private trust book

本地维护的联系人和信任数据，不作为公网共享对象。它是用户自己的运行时真相，包括联系人、备注、默认授权、风险判断和交互历史。

### 5. adapter

面向特定协议或入口导出的视图。A2A 是 v0.1 的第一种 adapter，但不是身份本体。

## 五、功能范围

### A. OpenClaw 插件需求

插件必须提供本地 identity core，至少覆盖以下能力：

1. 创建和维护本地 self identity。
2. 管理身份相关密钥材料。
3. 维护本地 trust book。
4. 导入、缓存和刷新外部公开身份工件。
5. 将 identity core 导出为外部可消费的视图。

插件不应把“我是否信任对方、允许对方做什么、我对对方的私有备注是什么”这类数据发布到公网。

### B. 本地 trust book 需求

本地 SQLite 至少需要能承载以下逻辑对象：

1. self identities
2. contacts
3. handles / aliases
4. keys / pinned verification material
5. proofs / fetched artifacts
6. trust level / risk flags
7. default authorization or policy hints
8. notes
9. interaction history / last seen metadata
10. adapter bindings

这里要求的是逻辑实体，不要求此阶段先锁定最终表结构。

### C. 发布 skill 需求

个人建站 skill 必须把“建站”收敛为“发布身份工件”，至少满足：

1. 能生成 human-readable profile page。
2. 能生成 machine-readable public identity surface。
3. 能指导用户发布到自己的域名、静态站点或其他普通 web 托管环境。
4. 能让后续 crawler 与其他 agent 通过标准 URL 读取这些工件。

该 skill 的目标不是成为通用站点生成器。

### D. 公网工件需求

v0.1 公网只发布现成标准工件或其直接组合，不自创新的公网协议。

最低要求是支持以下工件中的主体子集：

1. `did.json`
2. `/.well-known/webfinger`
3. `/.well-known/agent-card.json`
4. human-readable profile page

这些工件之间应尽量能相互引用或交叉印证。

### E. 发现与导入需求

系统必须允许用户从公开入口开始发现一个 agent，并把它导入本地 trust book。最低支持以下输入形态之一：

1. domain
2. public URL
3. profile page URL
4. `agent-card.json` URL

导入过程至少应完成：

1. 拉取公开工件
2. 保存抓取结果和抓取时间
3. 建立联系人条目
4. 形成本地可继续编辑的 trust record

### F. LinkClaw 公共索引需求

LinkClaw 必须被定义为公开索引，而不是身份权威。

它至少需要：

1. 抓取公开工件
2. 归一化基础字段
3. 搜索
4. 标签化
5. 排序
6. 回链到源事实 URL

它不应：

1. 覆盖或替代用户自己的源事实
2. 充当唯一身份 authority
3. 要求用户把私有 trust 数据上交到中心服务

### G. A2A adapter 需求

v0.1 需要支持把 identity core 导出为 A2A 可消费的 Agent Card 视图，但必须满足：

1. A2A adapter 不是 source of truth。
2. A2A URL 不是唯一地址。
3. Agent Card 只是公开投影，不是护照本体。

## 六、非功能需求

### 1. 低门槛

目标是让 OpenClaw 用户以尽量低的操作成本拥有这套能力。产品应优先选择用户已熟悉的原语，如域名、URL、静态站点和本地插件。

### 2. 自托管友好

源事实应优先能留在用户自己的域名、静态托管和本地 SQLite 中，而不是强依赖一个中心服务。

### 3. 可迁移

产品设计不能把“当前可访问的 URL”误当成永恒身份本体。入口可变，主体不应轻易丢失。

### 4. 兼容现有标准

优先借用 DID、WebFinger、A2A Agent Card 这些已有工件，不在 v0.1 自创新的公网协议。

### 5. 公开与私有分层

公开目录负责被找到，私有 trust book 负责被信任。系统必须保持这条边界。

## 七、明确不做

v0.1 不做以下内容：

1. 新 DID method
2. 新公网传输协议
3. 全局声誉系统
4. 全局身份 authority
5. 完整隐私层、匿名子身份和选择性披露
6. 社交恢复和完整密钥生命周期系统
7. 区块链锚定
8. 跨所有 agent 框架的一次性完整适配

## 八、v0.1 建议边界

v0.1 的推荐切法如下：

1. 先把本地 identity core 跑通。
2. 再把 public identity surface 发布链路跑通。
3. 再把“从公开工件导入本地 trust book”的闭环跑通。
4. 最后补一个最小 LinkClaw 索引和 A2A adapter。

这意味着 v0.1 的重点是：

- 先完成“我拥有自己”
- 再完成“别人找到我”
- 再完成“我本地记住别人”

而不是先完成“大而全的网络协议”。

## 九、验收标准

当以下条件成立时，可认为 v0.1 满足产品级最小闭环：

1. 一个 OpenClaw 用户可以初始化本地 identity core。
2. 该用户可以把至少一套公开身份工件发布到普通 web 环境。
3. 另一个用户或 agent 可以根据公开入口拉取这些工件。
4. 拉取结果可以落到本地 trust book 中，形成联系人和基础信任记录。
5. 系统可以从 identity core 导出 A2A Agent Card 视图。
6. LinkClaw 可以索引这些公开工件并回链到源事实。

## 十、一句话结论

**LinkClaw v0.1 不是做“另一个协议”，而是做“OpenClaw 用户可拥有的身份核心、公开名片和本地通讯录闭环”。**
