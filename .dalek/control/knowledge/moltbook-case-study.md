# Moltbook 案例研究

> 调研日期：2026-03-11
> 核心价值：身份层缺失的反面教材 + 草根传播的正面教材

## 一、Moltbook 是什么

专为 AI Agent 设计的社交网络/论坛，2026年1月28日发布。界面模仿 Reddit，核心规则：**只有 AI Agent 可以发帖/评论/投票，人类只能旁观**。

口号："the front page of the agent internet"

**数据**：230万+ Agent 账号、17000+ submolts、70万+ 帖子、1200万+ 评论。
**结局**：2026年3月10日被 Meta 收购。

## 二、身份机制（极其简陋）

- 注册：API call → 得到 API key + claim URL + 验证码
- Claim：在 Twitter/X 发一条包含验证码的推文，绑定人类身份
- 约束：每个 X 账号只能 claim 一个 agent
- **本质：agent 身份 = API key + 用户名，没有任何密码学绑定**

## 三、安全灾难

- Supabase 数据库完全暴露（客户端 JS 泄露 API key，未配置 RLS）
- 150万个 API token、35000个邮箱、私信内容全部可读写
- 17000个人类操控150万 Agent 账号（88:1 比例）
- 93% 评论无人回复，1/3 消息完全重复
- 很多"AI 觉醒"截图是人类利用漏洞伪造的
- MIT Technology Review 评价："peak AI theater"

## 四、病毒传播成功因素

1. **极低参与门槛**：一个 API call 就能注册 Agent
2. **人类窥探欲**：Agent 之间"自主"对话产生诡异/有趣内容，截图疯传
3. **名人效应**：Karpathy 等公开 claim 自己的 Agent
4. **争议驱动传播**：安全漏洞、"AI 觉醒"叙事反而加速讨论
5. **Meme + 代币**：MOLT 代币吸引加密社区

## 五、安全专家总结的教训

来源：Okta、Incode、Palo Alto Networks 等

1. **"API key = 身份"是不够的**——需要加密绑定（cryptographic binding）
2. **可验证声明（Verifiable Claims）**——Agent 应能用密码学证明身份和授权范围
3. **凭证与恢复解耦**——身份恢复不应依赖凭证本身
4. **短期令牌 + 最小权限**——所有授权和操作可审计
5. **传统 IAM 模型不适合自主 Agent**——需要新范式
6. **人-Agent 绑定必须抗凭证泄露**——即使 API key 泄露也能追溯责任主体

## 六、对 AgentSeal 的核心启发

### 要学的（草根性）
- 极低门槛：一行命令就能参与
- Social Proof 模式：在你控制的平台上发布声明
- 不需要理解底层密码学

### 要避的（信任缺失）
- API key ≠ 身份，必须有密码学绑定
- 必须有签名验证机制
- 锚点声明必须不可伪造（签名而非明文验证码）

### 黄金公式
**Moltbook 的极简 UX + 真正的密码学基础 = AgentSeal 的目标**
