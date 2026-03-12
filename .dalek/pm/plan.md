# PM Plan

## 项目目标
- 目标：构建 AgentSeal —— 一个极简的 AI Agent 身份签名验证工具
- 背景：OpenClaw 等 AI Agent 生态爆发，分布式智能体网络缺乏去中心化身份层。现有方案（MCP-I/DID/ERC-8004）走标准化/企业路线，缺少草根级、开发者友好的工具
- 成功标准：使用门槛 ≤ ssh-keygen，5 个 CLI 命令覆盖核心功能

## 范围与边界
- In Scope:
  - Ed25519 密钥对生成与管理
  - agentseal.json 身份文档格式
  - Social Proof 锚点发布（GitHub/DNS/个人网站）
  - 消息签名与验证
  - "名片 URL" 握手协议
  - 信任缓存（known-seals）
- Out of Scope（后续扩展）:
  - 声誉系统
  - 匿名子身份/选择性披露
  - 委托授权链
  - 区块链锚定
  - OpenClaw skill 集成（Phase 2）
- 关键约束：
  - PM 只负责拆解、调度、验收和 merge，不直接实现产品代码
  - 如果 merge 在产品文件上产生冲突，PM 必须 abort merge 并创建 integration ticket

## 阶段计划
| Phase | 目标 | 状态 | 验收标准 |
| --- | --- | --- | --- |
| phase-0 | 调研与设计 | completed | 生态调研完成，设计文档就绪 |
| phase-1 | CLI MVP | pending | agentseal init/publish/sign/verify/known 五个命令可工作 |
| phase-2 | OpenClaw 集成 | pending | 作为 OpenClaw skill/plugin 可安装使用 |

## 执行快照
- 当前阶段：phase-0 已完成，准备进入 phase-1
- 下一步：确定技术栈，创建项目骨架，开始实现 CLI
- 最近更新：2026-03-11 完成全部调研和设计文档

## 风险与阻塞
- 风险：赛道已拥挤（MCP-I/ERC-8004 等），需要靠极简差异化
- 阻塞：暂无硬阻塞；用户态初始化已于 2026-03-12 完成，接下来主要缺少 CLI 实现骨架
- 依赖：无外部依赖，纯本地工具

## 决策记录
- 2026-03-11: 确定"名片 URL"握手模型，放弃复杂协议设计
- 2026-03-11: 定位为草根工具而非标准，与 MCP-I 互补
- 2026-03-11: MVP 先做信任/公开端，隐私/匿名后续扩展
- 2026-03-11: Moltbook 案例确认"极简 UX + 密码学"方向

## 知识文档索引
- .dalek/control/knowledge/openclaw-ecosystem.md — OpenClaw 生态调研
- .dalek/control/knowledge/identity-layer-landscape.md — 身份层方案调研
- .dalek/control/knowledge/moltbook-case-study.md — Moltbook 案例研究
- .dalek/control/knowledge/agentseal-design.md — AgentSeal 设计文档
- .dalek/control/knowledge/linkclaw-product-vision.md — LinkClaw 产品愿景与当前 User Story
- .dalek/control/knowledge/linkclaw-requirements.md — LinkClaw v0.1 需求文档
- .dalek/control/knowledge/linkclaw-v0-architecture.md — LinkClaw v0 系统设计

## 复盘
- 做得好的地方：多路并行调研高效，deepdive 发现关键转折点
- 待改进：初期过度 PM 化（找切入点/竞品分析），用户纠正后回归"建能力"本质
- 后续行动：进入 phase-1 实现
