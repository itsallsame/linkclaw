# LinkClaw v0 系统设计

> 创建日期：2026-03-12
> 状态：draft v0.1
> 依据：linkclaw-product-vision.md / linkclaw-requirements.md

## 一、设计目标

这份系统设计只回答一件事：

**如何把 LinkClaw 落成一套可实现的 v0 系统，而不是停留在产品概念。**

v0 的设计目标有五个：

1. 让 OpenClaw 用户拥有一个独立于平台账号、channel id 和 service endpoint 的长期身份根。
2. 让该身份根可以被发布为普通 Web 可读取的公开工件。
3. 让别的用户或 agent 可以从公开入口解析、验证并导入该身份。
4. 让公开身份导入后可以沉淀为本地 trust book，而不是停留在一次性验签。
5. 让 A2A 之类协议消费 identity core 的导出视图，而不是反过来成为 source of truth。

## 二、系统边界

LinkClaw v0 不是单一程序，而是一组有明确边界的组件：

1. `Identity Core`：运行在 OpenClaw 本地，保存 self identity、keys、trust book。
2. `Publishing Skill`：把本地 identity core 编译成公开工件，并帮助发布到普通 Web 托管环境。
3. `Public Identity Surface`：用户控制的域名或静态站点，承载 `did.json`、`WebFinger`、`agent-card.json` 和 profile page。
4. `LinkClaw Index`：抓取和索引公开工件，只做搜索和回链，不持有源事实。
5. `Adapters`：A2A、IM、email、HTTP 等入口消费 identity core 导出的视图。

系统边界的核心原则是：

- 本地是源事实
- 公网是公开投影
- 索引是派生缓存
- 协议入口是适配层

## 三、源事实与派生视图

v0 必须先把 source of truth 说清楚。

### 1. 源事实

以下内容只能以本地 identity core 为准：

1. self identity
2. 当前有效密钥与轮换关系
3. 联系人、本地备注、trust level、risk flags
4. 默认授权和策略提示
5. interaction history

### 2. 公开投影

以下内容是本地 identity core 的公开投影：

1. `/.well-known/did.json`
2. `/.well-known/webfinger`
3. `/.well-known/agent-card.json`
4. human-readable profile page

### 3. 派生缓存

以下内容都不是源事实：

1. LinkClaw Index 中的索引记录
2. 对端本地抓取到的 artifact snapshots
3. A2A Agent Card 导出视图
4. IM / email / HTTP 中附带的 identity URL 或 signed envelope

### 4. canonical identity 合同

为避免不同实现各自产生不同的 principal id，v0 对 `canonical_id` 做以下强约束：

1. `canonical_id` 必须等于当前公开 `did.json` 中的 `id`
2. 默认 domain-first 场景下，推荐 `canonical_id` 采用 `did:web:<normalized-host>` 或其 path 变体
3. `self_id`、`contact_id` 只作为本地数据库主键，不对外暴露，也不能替代 `canonical_id`
4. `handles`、`webfinger` links、`agent-card`、profile page 和高价值动作 envelope 中的 `issuer`，最终都必须能回指同一个 `canonical_id`

状态推进约束：

1. 只有拿到可解析的 `did.json` 并确认其中 `id` 后，contact 才能进入 `consistent`
2. 只有 profile page、agent card 或 WebFinger，但还未解析到 `did.json` 时，最高只能进入 `resolved`

## 四、分层架构

```text
                +---------------------------+
                |     LinkClaw Index        |
                | search / normalize / link |
                +-------------+-------------+
                              ^
                              |
                +-------------+-------------+
                |   Public Identity Surface |
                | did.json / webfinger      |
                | agent-card / profile      |
                +-------------+-------------+
                              ^
                              |
                +-------------+-------------+
                |      Publishing Skill     |
                | render / package / check  |
                +-------------+-------------+
                              ^
                              |
                +-------------+-------------+
                |       Identity Core       |
                | self / keys / trust book  |
                | resolver / verifier       |
                | adapter exporter          |
                +------+------+-------------+
                       |      |
                       |      +----------------------+
                       |                             |
             +---------+---------+         +---------+---------+
             |     A2A Adapter    |         | IM / Email / HTTP |
             | agent card export  |         | identity URL / sig |
             +--------------------+         +--------------------+
```

## 五、核心模块设计

### A. Identity Core

Identity Core 是 v0 的绝对内核，建议拆成六个模块：

1. `Self Identity Manager`
   - 创建 self identity
   - 绑定 canonical id、display name、description、home origin

2. `Key Manager`
   - 生成密钥
   - 标记 active / retired / revoked
   - 维护 key rotation chain

3. `Artifact Compiler`
   - 把本地 identity record 编译成公开工件
   - 保证各工件字段一致并相互引用

4. `Resolver / Fetcher`
   - 从 domain、profile URL、agent card URL 等输入开始解析候选工件
   - 拉取并缓存 artifacts

5. `Verifier`
   - 校验工件间引用关系
   - 校验 key consistency
   - 校验 proof freshness 和 fetch status

6. `Trust Book`
   - 保存 contacts、trust、notes、policy、interaction history
   - 把一次解析结果转化为长期关系对象

### B. Publishing Skill

Publishing Skill 负责把“有身份”变成“能被别人找到”。

它至少应负责：

1. 生成一组发布工件
2. 生成目录结构与默认 URL
3. 检查引用是否自洽
4. 指导部署到域名、静态站点或托管环境
5. 发布后做一次公开自检

### C. LinkClaw Index

Index 只做三件事：

1. 抓取公开工件
2. 归一化可搜索字段
3. 回链到源事实 URL

它不做：

1. 存储私有 trust book
2. 取代用户域名
3. 成为唯一 authority

### D. Adapters

Adapters 的统一原则是：

- 引用 identity core
- 不篡改 identity root
- 不承载私有 trust truth

v0 首先定义 `A2A Adapter`，后续再扩展 IM、email、HTTP。

## 六、本地逻辑数据模型

v0 不先锁死 SQL 表结构，但必须锁定逻辑实体。

### 1. `self_identities`

表示“我拥有的龙虾是谁”。

关键字段建议：

- `self_id`
- `canonical_id`
- `display_name`
- `description`
- `home_origin`
- `default_profile_url`
- `status`

### 2. `handles`

表示公开入口和别名。

关键字段建议：

- `handle_id`
- `owner_type` (`self` / `contact`)
- `owner_id`
- `handle_type` (`domain` / `url` / `acct` / `alias`)
- `value`
- `is_primary`

### 3. `keys`

表示身份验证材料。

关键字段建议：

- `key_id`
- `owner_type`
- `owner_id`
- `algorithm`
- `public_key`
- `private_key_ref`
- `status`
- `published_status`
- `rotates_from`
- `valid_from`
- `valid_until`
- `retired_at`
- `revoked_at`
- `grace_until`

### 4. `published_artifacts`

表示本地生成过的公开工件。

关键字段建议：

- `artifact_id`
- `self_id`
- `artifact_type`
- `url`
- `content_hash`
- `generated_at`
- `last_verified_at`

### 5. `contacts`

表示导入到本地 trust book 的对端主体。

关键字段建议：

- `contact_id`
- `canonical_id`
- `display_name`
- `home_origin`
- `profile_url`
- `status`
- `last_seen_at`

### 6. `artifact_snapshots`

表示抓取到的对端公开工件快照。

关键字段建议：

- `snapshot_id`
- `contact_id`
- `artifact_type`
- `source_url`
- `fetched_at`
- `http_status`
- `content_hash`
- `parsed_summary`

### 7. `proofs`

表示公开背书、交叉引用或外部证明。

关键字段建议：

- `proof_id`
- `contact_id`
- `proof_type`
- `proof_url`
- `observed_value`
- `verified_status`
- `verified_at`

### 8. `pinned_materials`

表示本地显式 pin 住的验证材料。

关键字段建议：

- `pin_id`
- `contact_id`
- `material_type`
- `material_value`
- `reason`
- `pinned_at`

### 9. `policy_hints`

表示本地默认授权和策略提示。

关键字段建议：

- `policy_id`
- `owner_type`
- `owner_id`
- `adapter_type`
- `intent`
- `default_action`
- `note`
- `updated_at`

### 10. `trust_records`

表示本地信任判断。

关键字段建议：

- `trust_id`
- `contact_id`
- `trust_level`
- `risk_flags`
- `verification_state`
- `decision_reason`
- `updated_at`

### 11. `notes`

表示本地备注。

关键字段建议：

- `note_id`
- `contact_id`
- `body`
- `created_at`

### 12. `interaction_events`

表示长期关系历史。

关键字段建议：

- `event_id`
- `contact_id`
- `channel`
- `event_type`
- `summary`
- `event_at`

### 13. `adapter_bindings`

表示某个 contact 在特定协议中的入口映射。

关键字段建议：

- `binding_id`
- `contact_id`
- `adapter_type`
- `endpoint`
- `external_id`
- `last_verified_at`

## 七、公开工件设计

v0 不自创新公网协议，但要把现有工件编译成一组一致的 surface。

### 1. `/.well-known/did.json`

作用：

- 提供 domain-first 的长期 identity document
- 承载 verification methods 和 service references

最关键的字段是：

- stable id
- verification methods
- also known handles
- service endpoints

### 2. `/.well-known/webfinger`

作用：

- 提供从 handle 或公开地址到身份资源的发现入口
- 把“人类知道的地址”导向“机器读取的工件”

最关键的字段是：

- subject
- links 到 profile / did / agent card

v0 的 WebFinger 归一化规则是：

1. 必须支持 `resource=https://<origin>/` 这一种统一入口
2. 如果 owner 额外提供 `acct:` 形态，可作为 alias，但不是唯一入口
3. bare domain 导入时，Resolver 必须先归一化为 `https://<origin>/`，再发起 WebFinger 查询
4. 如果 WebFinger 返回的 did / agent card / profile 与当前 origin 无法互相印证，状态不能超过 `resolved`

### 3. `/.well-known/agent-card.json`

作用：

- 作为 A2A 等协议消费的公开 adapter 视图

最关键的字段是：

- agent name
- description
- capabilities summary
- service endpoint
- auth requirements
- reference back to canonical identity

### 4. human-readable profile page

作用：

- 给人读
- 充当 social proof surface
- 提供机器工件的入口导航

页面至少要链接到：

- did document
- agent card
- 可选的社交背书入口

### 5. 工件一致性规则

所有公开工件都应尽量满足：

1. 指回同一个 canonical identity
2. 引用同一组当前有效 keys 或 key references
3. 指向同一个 home origin
4. 能互相导航

### 6. 工件职责与优先级

为避免多个公开工件互相打架，v0 固定以下 precedence：

1. `did.json` 对 `canonical_id`、verification keys 和 key status 有最高权威
2. `webfinger` 只负责 discovery aliases 和 links，不覆盖 `canonical_id`
3. `agent-card.json` 只对 A2A service endpoint、capability summary、auth requirements 有权威，不覆盖 identity root
4. profile page 只提供人类可读信息和 social proof，不覆盖 key material 或 `canonical_id`

冲突处理规则：

1. 如果 `did.json` 与 `agent-card.json` 的 key references 冲突，状态直接进入 `mismatch`
2. 如果 profile page 的文案与 did / agent card 不一致，profile 只能降级为低可信展示层，不能覆盖结构化工件
3. 如果缺失 `did.json`，其余工件最多只能把状态推进到 `resolved`

### 7. 最小可用工件组合

v0 允许公开工件是主体子集，但不同组合有不同上限：

1. 只有 `did.json`：可以进入 `resolved`
2. `did.json` + (`webfinger` 或 `agent-card.json` 或 profile page) 且交叉引用一致：可以进入 `consistent`
3. 只有 `agent-card.json`：可以进入 `discovered`，必须继续追到 did 才能升级
4. 只有 profile page：可以进入 `discovered`，不能直接形成 `consistent`
5. 只有 WebFinger：可以进入 `discovered`，必须继续拉取 links 指向的工件

### 8. key rotation / revocation 公开规则

v0 不额外发明新协议，但要求当前公开 `did.json` 明确表达 key lifecycle 元数据。

规则如下：

1. `did.json` 必须列出当前可接受的 verification keys
2. 每个公开 key entry 至少要能表达 `published_status`、`valid_from`，以及在需要时表达 `retired_at`、`revoked_at`、`grace_until`
3. `agent-card.json` 只引用当前 active keys，不引用已 revoked keys
4. 导入方验证 envelope 时，只能接受以下两类 key：
   - `active`
   - `retired` 且 `issued_at <= grace_until`
5. `revoked` key 不能再用于新的信任提升；历史事件可保留为审计记录，但不能自动提升 trust level
6. 如果导入方只看到了新 key、没看过旧 snapshots，则旧 key 的历史 envelope 最多降级为 `seen`，不能直接判定为 `verified`

## 八、核心流程设计

### 流程 1：初始化 self identity

1. 用户在 OpenClaw 中启用 LinkClaw
2. Identity Core 创建 `self_identity`
3. Key Manager 生成初始密钥
4. 用户绑定主 domain 或主 profile URL
5. Artifact Compiler 生成公开工件 bundle
6. Publishing Skill 接管发布

输出：

- 一条本地 self identity
- 一组可发布工件
- 一个可分享的公开入口

### 流程 2：发布公开身份

1. Publishing Skill 渲染 `did.json`
2. 渲染 `webfinger`
3. 渲染 `agent-card.json`
4. 渲染 profile page
5. 部署到目标 origin
6. 公开自检：逐个 URL 拉回检查可访问性和引用一致性

输出：

- 一组可公开读取的 URL
- 一次发布记录

### 流程 3：导入并识别别人

输入可以是：

- domain
- public URL
- profile page URL
- agent-card URL

处理步骤：

1. Resolver 先把输入归一化为 origin、candidate handle 和可用的 WebFinger resource
2. 推导候选工件 URL
3. 并行抓取公开工件
4. 优先解析 `did.json`，确认 `canonical_id`
5. 按 precedence 比较 canonical id、origin、key references、cross links
6. 根据“最小可用工件组合”决定状态最多推进到 `discovered / resolved / consistent`
7. 形成 `contact`
8. 保存 `artifact_snapshots`
9. 初始化 `trust_record`

输出：

- 一个本地 contact
- 一组可追溯抓取记录
- 一个当前 verification state

### 流程 4：关系沉淀

1. 后续交互进入 `interaction_events`
2. 用户更新 notes、risk flags、trust level
3. 联系人的公开工件可周期性刷新
4. 若发现 key mismatch、artifact drift、profile 失联，更新风险状态

输出：

- 从“我见过它”进化到“我持续认识它”

### 流程 5：导出 A2A adapter

1. Adapter Exporter 读取 self identity
2. 读取当前有效 keys、service endpoint、capability summary
3. 生成 A2A Agent Card
4. 把 Agent Card 发布到公开 surface

关键约束：

- A2A card 必须回指 canonical identity
- endpoint 可变，但 canonical identity 不能被 endpoint 替代

### 流程 6：高价值动作的 signed envelope

v0 不要求所有闲聊消息都签名，但要为高价值动作预留统一 envelope。

建议字段：

- `issuer`
- `audience`
- `issued_at`
- `expires_at`
- `message_id`
- `nonce`
- `intent`
- `content_type`
- `hash_algorithm`
- `payload_hash`
- `key_id`
- `signature`

生成规则：

1. `issuer` 必须等于 `canonical_id`
2. `payload_hash` 默认使用 `SHA-256`
3. 签名前必须先构造“无 signature 字段”的 envelope
4. envelope 使用稳定的 canonical JSON 序列化，再以 UTF-8 字节串作为签名输入
5. `signature` 只覆盖 envelope 本身；payload 通过 `payload_hash` 间接绑定

验签顺序：

1. 解析 `issuer`
2. 解析并获取对应 contact / self identity 的有效 key set
3. 检查 `issued_at` / `expires_at`
4. 检查 `message_id` 或 `nonce` 是否重放
5. 重算 `payload_hash`
6. 验证 envelope signature
7. 只有验签通过后，才进入本地 trust / policy 判断

建议用途：

- 授权
- 敏感数据交换
- 支付或交易确认
- 对外 API 调用

## 九、身份解析与验证规则

v0 的验证重点不是“做全网裁判”，而是“把公开可见事实沉淀为本地可判断事实”。

建议验证状态分为四层：

1. `discovered`
   - 找到了公开入口，但尚未完成交叉校验

2. `resolved`
   - 拉到了足够工件，能形成 contact

3. `consistent`
   - 工件间引用一致，没有明显冲突

4. `mismatch`
   - key、origin、canonical id 或 cross links 出现冲突

信任等级则独立存在，例如：

- `unknown`
- `seen`
- `verified`
- `trusted`
- `pinned`

这样可以避免把“技术上一致”误当成“业务上信任”。

artifact precedence 的具体实现规则是：

1. `did.json` 决定 `canonical_id`
2. `did.json` 决定当前可接受 key set
3. `webfinger` 决定 alias 和可跳转 links
4. `agent-card.json` 决定 A2A endpoint 和 capability summary
5. profile page 只能补充 label、description、social proof

如果不同来源对同一字段给出不同值，按上述顺序择高；被低优先级工件覆盖的值只能作为备注或风险信号保存。

## 十、LinkClaw Index 设计

Index 只消费公开 surface，不消费私有 trust book。

### 1. 抓取输入

- seed domains
- seed profile URLs
- 用户主动提交的公开入口

### 2. 索引输出

- canonical label
- canonical id
- description
- handles
- source URLs
- tags
- content hashes
- freshness metadata
- conflict state

### 3. 约束

Index 返回的每条记录都必须带源事实链接，并且明确说明：

- 这是索引视图
- 不是身份 authority
- 最终验证应以源站和本地 trust book 为准

冲突处理规则：

1. 同一 handle 指向多个 `canonical_id` 时，Index 不得自动 merge，必须进入 `conflict state`
2. 同一 `canonical_id` 被多个 origin 声称时，Index 只可把“其来源域名/路径与 `canonical_id` 可还原出的 did:web 位置一致”的记录标为 primary，其余标为 competing claims
3. conflict 记录可以展示，但必须降权且显式标红
4. Index 不得把来自 profile page 的弱信号覆盖结构化工件的强信号

## 十一、安全与运维边界

### 1. 密钥管理

- 私钥只留本地
- 公钥通过公开工件发布
- 轮换必须保留前后关系
- 公开 key lifecycle 必须能从当前 `did.json` 中恢复

### 2. 工件刷新

- artifact snapshots 应保存抓取时间
- 旧快照不覆盖历史
- mismatch 要可审计
- snapshots 应作为旧 key / 旧 endpoint 的历史证据来源

### 3. 默认信任

- 导入 contact 不等于自动信任
- consistency 不等于 trusted
- 高风险动作必须走显式判断或 envelope 验证

### 4. 故障处理

以下情况不应静默通过：

- did 与 agent card 指向不同 key
- profile page 指向不同 origin
- endpoint 改变但没有新工件对应
- proofs 过期或失联

## 十二、v0 实现边界

v0 必须实现：

1. 本地 identity core
2. 一组最小公开工件
3. 从公开工件导入 contact 的闭环
4. A2A Agent Card 导出
5. 最小 LinkClaw Index

v0 不实现：

1. 全局声誉
2. 复杂授权链
3. 匿名子身份体系
4. 跨所有 agent 框架的一次性深度适配
5. 全量消息默认签名

## 十三、一句话架构结论

**LinkClaw v0 的系统设计不是“再造一个 agent 协议”，而是“以本地 identity core 为源事实，把 Web 工件、A2A 视图和本地 trust book 收成一个闭环”。**
