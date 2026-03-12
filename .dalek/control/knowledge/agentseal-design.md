# AgentSeal 设计文档

> 创建日期：2026-03-11
> 状态：概念设计阶段

## 一、定位

给 AI Agent 签名验证的极简开发者工具。不是身份标准，是可工作的工具。

**设计哲学**：
- Code first, standardize later（Git/Docker 路线，非 DID/W3C 路线）
- 使用门槛 ≤ ssh-keygen
- 不需要区块链、不需要服务器、不需要账号
- 用户不需要理解 DID/VC/PKI

**与 MCP-I 的关系**：互补不竞争。AgentSeal 是草根路线（Keybase 模式），MCP-I 是企业级标准化路线。

## 二、核心概念：三层模型

```
Layer 0: 密钥对（身份根）
  └── Ed25519 密钥对，本地生成，本地保管

Layer 1: 身份文档（公开声明）
  └── agentseal.json：公钥 + 锚点列表 + 能力声明
  └── 托管在你控制的地方（GitHub/个人网站/IPFS）

Layer 2: 签名验证（运行时使用）
  └── Agent 发言/操作时签名
  └── 对方验证签名 → 查找身份文档 → 验证锚点
```

## 三、握手协议："名片 URL" 模型

**核心思路：你的身份 = 一个 URL**

- `https://github.com/xiewanpeng/xiewanpeng/agentseal.json`
- `https://xiewanpeng.dev/.well-known/agentseal.json`
- `https://gist.github.com/xiewanpeng/abc123`

**握手 = 交换 URL**

```
Agent A: "我是 https://xiewanpeng.dev/.well-known/agentseal.json"
Agent B: fetch(url) → 拿到公钥 → 验证签名 → OK
Agent B: "我是 https://lisi.dev/.well-known/agentseal.json"
Agent A: fetch(url) → 拿到公钥 → 验证签名 → OK
```

**为什么选这个模型**：
1. 零协议设计：用 HTTP 就行
2. 零概念负担：URL 人人都懂
3. 零基础设施：GitHub 就是"身份服务器"
4. 自然兼容现有 channel：在 Discord/微信里发一个 URL 就是握手
5. 人类也能验证：浏览器打开 URL 就能看到身份文档

**信任缓存**（类似 SSH known_hosts）：
```json
// ~/.agentseal/known-seals.json
{
  "https://lisi.dev/.well-known/agentseal.json": {
    "publicKey": "z6Mkf5rGz...",
    "verifiedAt": "2026-03-11T10:00:00Z",
    "trustLevel": "high",
    "proofs": {"github": true, "dns": true}
  }
}
```

## 四、身份文档格式（agentseal.json）

```json
{
  "version": "0.1",
  "id": "seal:ed25519:z6Mkf5rGz...",
  "publicKey": "z6Mkf5rGz...",
  "algorithm": "Ed25519",
  "owner": {
    "name": "xiewanpeng",
    "description": "Builder of things"
  },
  "proofs": [
    {
      "type": "github",
      "handle": "xiewanpeng",
      "url": "https://github.com/xiewanpeng",
      "claim": "GitHub Profile README 或 gist 中包含 seal ID"
    },
    {
      "type": "dns",
      "domain": "xiewanpeng.dev",
      "claim": "DNS TXT _agentseal.xiewanpeng.dev 包含公钥"
    }
  ],
  "created": "2026-03-11T00:00:00Z",
  "expires": "2027-03-11T00:00:00Z"
}
```

## 五、CLI 命令设计

```bash
agentseal init              # 生成密钥对 + agentseal.json
agentseal publish           # 引导发布到 GitHub/个人网站
agentseal sign <message>    # 签名消息
agentseal verify <url> <sig> # fetch 身份文档 + 验证签名
agentseal known             # 管理已验证的信任列表
```

## 六、使用场景

### 场景 1：两个 Agent 在 IM 群里握手
```
Agent A: 你好！我的身份：https://xiewanpeng.dev/.well-known/agentseal.json
Agent B: [自动 fetch → 验证] ✅ 已验证：xiewanpeng (GitHub ✅, DNS ✅)
```

### 场景 2：人验证 Agent 身份
```
我：你是张三的 Agent 吗？
Agent：是的，我的身份印章：https://zhangsan.dev/.well-known/agentseal.json
我：[浏览器打开 → 核实 GitHub] → OK
```

### 场景 3：Agent API 调用认证
```http
POST /api/translate
Authorization: AgentSeal https://xiewanpeng.dev/.well-known/agentseal.json
X-Seal-Signature: <签名>
X-Seal-Timestamp: 2026-03-11T10:00:00Z
```

## 七、设计约束

1. **零配置**：init 之后就能用
2. **零依赖**：不需要区块链/服务器/账号
3. **零概念负担**：不需要理解 DID/VC/PKI
4. **可渐进发现**：高级功能存在但不强制

## 八、扩展路径（后续，非 MVP）

1. 授权委托（我授权我的 Agent 代表我做某些事）
2. 声誉积累（这个身份过去做了什么）
3. 选择性披露 / 匿名子身份（HD 密钥派生）
4. 多 Agent 组织身份（一个组织下多个 Agent）
5. 密钥社交恢复
6. OpenClaw skill 集成

## 九、深度思考中的关键转折点

1. **信任 vs 隐私张力**：发现两者天然矛盾 → 决定 MVP 先做信任端
2. **发现 MCP-I 已存在** → 重新定位为草根路线而非标准路线
3. **从"身份层"到"签名工具"** → 降低认知门槛
4. **Moltbook 反面教材** → 确认"极简 UX + 密码学"是正确方向
5. **"名片 URL" 握手模型** → 用 HTTP+URL 替代复杂协议，极简但完整

## 十、已知风险和待解决问题

1. **密钥丢失/泄露**：MVP 先做密钥轮换声明（旧密钥签名新密钥）
2. **锚点时效性**：缓存+TTL 策略
3. **跨框架互操作**：协议标准化（但 code first）
4. **商业可持续性**：Keybase 的教训——基础设施不能靠 VC
5. **消息格式兼容**：IM channel 中不能嵌入 JSON → 身份文档+首次验证+session 信任
