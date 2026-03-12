# OpenClaw 生态调研

> 调研日期：2026-03-11
> 来源：多轮 WebSearch 综合

## 一、OpenClaw 概况

OpenClaw（原 Clawdbot/Moltbot）是开源自托管个人 AI Agent 平台，由奥地利开发者 Peter Steinberger（PSPDFKit 创始人，公司曾以约1亿美元出售）于 2025年11月发布。

**核心数据**：
- GitHub Stars：28万+（超越 React 的 24.3万，史上增长最快）
- 技能生态：5,400+ 注册技能（ClawHub）
- 通信平台集成：50+
- 创始人于 2026年2月14日加入 OpenAI，项目转交独立开源基金会

**核心架构**：
- Gateway 网关：单一 Node.js 进程，统一认证 + 消息路由 + 通道隔离
- 多 Agent 工作区：每个 Agent 独立 prompt、技能、记忆
- SOUL.md：配置文件定义 AI 人格与行为
- ContextEngine：v2026.3.7 引入的插件化上下文管理（bootstrap → ingest → assemble → compact → afterTurn → prepareSubagentSpawn → onSubagentEnded）
- Heartbeat：cron 触发的 agentic loop，每 30 分钟唤醒 Agent 自主巡检
- 记忆系统：纯 Markdown 文件，日志层（memory/YYYY-MM-DD.md）+ 长期记忆（MEMORY.md），向量+BM25 混合检索
- Skills：纯 Markdown 指令文档，非编译代码

## 二、关键时间线

| 时间 | 事件 |
|------|------|
| 2025.11 | 以 Clawdbot 发布 |
| 2026.01.27 | 因 Anthropic 商标投诉改名 Moltbot |
| 2026.01.30 | 再次改名 OpenClaw |
| 2026.02 | 一周 200万访客，14.5万星 |
| 2026.02.14 | 创始人加入 OpenAI |
| 2026.03.07 | v2026.3.7 发布，引入 ContextEngine 插件 |

## 三、安全问题

- 安全审计发现 512 个漏洞，8 个严重级别
- CVE-2026-25253（CVSS 8.8）：允许远程执行任意命令
- ClawJacked 漏洞：任何网站可静默接管用户的 AI Agent
- 公网发现 3万+ 暴露实例
- 社区市场发现 1000+ 恶意插件
- Microsoft、CrowdStrike、Kaspersky、Cisco、Palo Alto 均发出安全警告

## 四、国内大厂产品矩阵

| 公司 | 产品 | 定位 |
|------|------|------|
| 腾讯 | QClaw | 一键安装+微信/QQ 直连 |
| 腾讯 | WorkBuddy | 企业级 AI Agent，2000+ 内部员工测试 |
| 字节跳动 | ArkClaw | 火山引擎云端 SaaS |
| 字节跳动 | 抖音助手 | UI-TARS 手机 Agent |
| 飞书 | 官方 OpenClaw 插件 | 企业协作 AI |
| 阿里 | 百炼 OpenClaw | 模型+部署一站式，9.9元/月 |
| 阿里 | CoPaw | Python 技术栈，3 条命令部署，成本 1/10 |
| 小米 | miclaw | 手机系统级 AI Agent |
| 华为 | 小艺 OpenClaw 模式 | 鸿蒙多端 Agent |
| 中关村科金 | PowerClaw | 企业级解决方案 |
| 网易有道 | LobsterAI | 消费级安全版 |
| Moonshot | Kimi Claw | 云原生 SaaS，5000+ 技能 |

## 五、国内社区生态

- openclaw-cn：淘宝镜像源一键安装
- OpenClawChinese：CLI+Dashboard 全汉化，每小时同步
- OpenClaw-Docker-CN-IM：飞书/钉钉/QQ/企微全预装 Docker
- 深圳龙岗"龙虾十条"：最高补贴 200 万元
- A股出现 OpenClaw 概念股

## 六、对 Dalek 的映射

| OpenClaw 概念 | Dalek 对应 |
|--------------|-----------|
| Gateway | daemon（HTTP API + 调度循环）|
| SOUL.md | agent-kernel.md + agent-user.md |
| Skills (Markdown) | .dalek/control/skills/ |
| Heartbeat loop | manager tick |
| Memory (Markdown files) | state.json + report 事件链 |
| ContextEngine hooks | dispatch context 编译 |
| Multi-Agent workspace | ticket = 独立 worktree + worker |
