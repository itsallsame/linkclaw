# LinkClaw Phase-2 产品设计

> 日期：2026-03-15
> 状态：draft

## 核心认知转变

Phase-1 造了原子工具（CLI 命令 + plugin 能力）。
Phase-2 要造的不是更多工具，而是**三条使用路径 + 社会身份证明**。

用户不关心 `linkclaw`。用户关心的是：
1. 我的 agent 出门别人认不认识它
2. 别人来找我的 agent 时我能不能判断对方可信
3. 对方怎么确认"我就是我"，而不是有人冒充

## Phase-2 目标

**让 AI agent 的身份像 HTTPS 一样透明——用户不需要理解底层，身份自然发生。**

## 身份结构模型

LinkClaw 身份不是一个孤立的 DID 标识符，而是一个**根锚点 + 社会证明链**的分层结构：

```
┌─────────────────────────────────────────────┐
│  根锚点（Root Anchor）                        │
│  你控制的域名 / 主页                           │
│  → 证明：技术控制权（"我拥有这个入口"）         │
│  → 载体：did.json + webfinger + agent-card    │
└──────────────────┬──────────────────────────┘
                   │
    ┌──────────────┼──────────────┐
    │              │              │
    ▼              ▼              ▼
┌────────┐  ┌──────────┐  ┌──────────┐
│ GitHub │  │ LinkedIn │  │ DNS TXT  │  ...更多平台
│ 已验证  │  │ 已验证   │  │ 已验证   │
└────────┘  └──────────┘  └──────────┘
    社会证明面（Social Proof Surface）
    → 证明：社会身份（"我就是现实中的那个人"）
    → 方式：双向锚点（平台上有指向根锚点的签名证据）
```

**设计原理**：

- **根锚点**是身份的技术根基——你控制域名就拥有身份。但单独的域名控制权不回答"你是谁"。
- **社会证明面**是身份的社会根基——每多一个平台反向确认你，可信度就增加一层。就像现实中，身份证只是底线，公司、学校、朋友的背书才让别人真正信任你。
- **交叉验证越多，冒充成本越高**——冒充一个域名容易，同时冒充域名 + GitHub + LinkedIn + DNS 很难。
- **渐进式**——你可以只有域名（最低可用），加上 GitHub（开发者可信），再加 LinkedIn（职业可信），逐步建立信任。

这个结构决定了 inspect 的信任判定不再只是技术状态（consistent/mismatch），而是一个**多维信任信号**：

```
inspect alice.example →
  域名控制权: ✓
  工件一致性: consistent
  社会证明:
    GitHub @alice — 已验证
    LinkedIn Alice Zhang — 已验证
    DNS TXT — 已验证
  信任建议: 高可信度（3/3 锚点验证通过）
```

## 一、社会身份锚点（Social Proof Anchors）

### 问题

`did:web:wanpengxie.pages.dev` 只证明"这个人控制了这个域名"。不证明"这个域名背后是 GitHub 上的 wanpengxie"。

当前 LinkClaw 页面单向链接到 GitHub/LinkedIn，但没有反向验证。任何人都可以注册域名冒充任何人。

### 解决方案

双向锚点验证，类似 Keybase 的证明机制：

#### 支持的锚点平台（v0.2 先做最有价值的）

| 平台 | 验证方式 | 优先级 |
| --- | --- | --- |
| GitHub | 在 bio 或特定 repo 中放置 proof 文本 | P0 |
| DNS TXT | 在域名 DNS 记录中添加 TXT 记录 | P0 |
| 个人网站 | 在页面中嵌入 `<link>` 或 meta 标签 | P1 |
| Twitter/X | 发布包含 proof 的推文 | P2 |
| LinkedIn | 添加到 About 或 Website 字段 | P2 |

#### 验证闭环

```
1. 用户: linkclaw anchor add github
   → 生成 proof 文本: "I am did:web:wanpengxie.pages.dev — proof: <签名>"
   → 提示用户: "请把这段文字放到 GitHub bio 或创建 gist"

2. 用户操作完成后: linkclaw anchor verify github
   → LinkClaw 拉取 GitHub profile/gist
   → 验证 proof 文本存在且签名正确
   → 记录到 did.json 的 alsoKnownAs + 本地 anchor 表

3. 别人 inspect 时:
   → 看到 anchors: [{platform: "github", handle: "wanpengxie", verified: true, verified_at: "..."}]
   → 这不是"他说他是 wanpengxie"，而是"GitHub 上的 wanpengxie 确认了这个身份"
```

#### 数据模型

```sql
CREATE TABLE social_anchors (
  anchor_id TEXT PRIMARY KEY,
  self_id TEXT NOT NULL REFERENCES self_identities(self_id),
  platform TEXT NOT NULL,        -- github, dns, twitter, website
  handle TEXT NOT NULL,           -- wanpengxie, wanpengxie.pages.dev
  proof_url TEXT,                 -- https://github.com/wanpengxie 或 gist URL
  proof_text TEXT NOT NULL,       -- 签名后的 proof 文本
  verified BOOLEAN NOT NULL DEFAULT FALSE,
  verified_at TEXT,
  created_at TEXT NOT NULL
);
```

#### CLI 命令

```bash
linkclaw anchor add <platform>       # 生成 proof 文本，指导用户放置
linkclaw anchor verify <platform>    # 拉取并验证 proof
linkclaw anchor ls                   # 列出所有锚点及验证状态
linkclaw anchor rm <platform>        # 移除锚点
```

#### 公开工件变化

- did.json 的 `alsoKnownAs` 增加已验证的平台 URL
- profile page 的社交链接从普通链接变成"已验证"标记
- 新增 `/.well-known/anchors.json`：列出所有锚点及验证证据

#### inspect 输出变化

```json
{
  "verification_state": "consistent",
  "anchors": [
    {
      "platform": "github",
      "handle": "wanpengxie",
      "proof_url": "https://gist.github.com/wanpengxie/...",
      "verified": true,
      "verified_at": "2026-03-15T..."
    }
  ]
}
```

## 二、三条使用路径

### 路径 1：Onboarding — 给 agent 创建身份

**当前体验**：用户需要理解 canonical_id、did:web、手动 init + publish + deploy。

**目标体验**：
```
Agent: "你的 agent 还没有公开身份。要设置一个吗？"
用户: "好"
Agent: "你有自己的域名吗？还是用默认的？"
用户: "用默认的"
Agent: → 自动 init（canonical_id 从域名推导）
       → 自动 publish
       → 自动 deploy 到 Cloudflare Pages
Agent: "搞定。你的公开身份：https://xxx.pages.dev"
Agent: "要不要关联你的 GitHub 账号增加可信度？"
用户: "好"
Agent: → linkclaw anchor add github
       → 引导用户操作
       → linkclaw anchor verify github
Agent: "GitHub 已验证。现在别人可以确认你就是 GitHub 上的 wanpengxie。"
```

**关键设计**：
- 用户不需要知道 `did:web` 是什么
- canonical_id 从域名自动推导
- 社交锚点作为 onboarding 的自然延伸，而不是独立功能

### 路径 2：Outbound — agent 出门自带身份

**当前体验**：用户需要手动执行 `/linkclaw-share`。

**目标体验**：
```
用户: "去联系 alice 的 agent"
Agent: → 对外发送消息时自动附带 agent-card URL
       → 对方 agent 可以验证身份
用户全程无感知。
```

**关键设计**：
- Agent runtime 在对外交互时自动注入身份链接
- 类似浏览器自动发送 TLS 证书，用户不需要手动操作
- 需要 agent runtime（Claude Code / OpenClaw）支持 outbound hook

### 路径 3：Inbound — 有人来找我的 agent

**当前体验**：被动发现已实现，但提示语是技术语言。

**目标体验**：
```
陌生 agent 发来消息
Agent: "收到来自 alice.example 的请求。
        身份验证：通过
        GitHub 认证：@alice（已验证）
        之前没有合作过。
        要接受吗？"
用户: "信任她"
Agent: → 自动 import + trust level=trusted
```

**关键设计**：
- 自然语言提示，不是技术术语
- 社交锚点信息作为信任判断的重要输入（"GitHub 认证"比"consistent"有说服力）
- 用户用自然语言决策（"信任她"），不是执行命令

## 三、init 体验改进

### 问题

`linkclaw init` 要求用户输入 `canonical_id`，一个大多数人不理解的技术概念。

### 改进

```bash
# 当前（技术导向）
linkclaw init --canonical-id "did:web:example.com"

# 改后（用户导向）
linkclaw init --domain example.com
# → 自动推导 canonical_id = did:web:example.com
# → 如果不提供 domain，提示 "你的域名是？（留空使用本地模式）"
```

新增 `--domain` 参数，deprecated 但保留 `--canonical-id`。
无域名时允许本地模式（生成 `did:key:` 格式的临时 ID），后续绑定域名时迁移。

## Ticket 拆分建议

| # | 标题 | 优先级 | 说明 |
| --- | --- | --- | --- |
| 1 | Social Anchors: GitHub + DNS 双向验证 | P0 | anchor add/verify/ls/rm + 0005_social_anchors.sql + proof 签名 + GitHub API 拉取验证 |
| 2 | init 体验改进: --domain 替代 --canonical-id | P0 | 用户不需要理解 did:web |
| 3 | inspect/publish 集成 anchors | P0 | inspect 输出包含锚点验证结果，publish 更新 profile 页面显示已验证标记 |
| 4 | Onboarding Flow: agent 引导式身份设置 | P1 | 对话式引导，自动 init + publish + deploy + anchor |
| 5 | Inbound Flow: 自然语言信任决策 | P1 | 收到身份时用自然语言提示，包含锚点信息 |
| 6 | 更多锚点平台: Twitter/LinkedIn/Website | P2 | 扩展锚点验证到更多平台 |

## 设计原则

1. **身份是行为，不是工具** — 用户不"使用身份功能"，身份在交互中自然发生
2. **社会证明 > 技术验证** — "GitHub 上的 wanpengxie 确认了"比"did.json consistent"对用户有意义
3. **渐进式信任** — 域名控制权是底线，社交锚点增加可信度，交互历史沉淀为关系
4. **零术语** — 用户界面不出现 did:web、canonical_id、webfinger 这些词
