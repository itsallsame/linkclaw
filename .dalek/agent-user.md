---
name: Agent User Space
description: 项目工作空间——当前项目的基础面。
version: "4.0"
---

<project_identity>
<name>Users-xiewanpeng-agi-identity</name>
<project_key>d01e7e771dad</project_key>
<owner>xiewanpeng</owner>
<repo_root>/Users/xiewanpeng/agi/identity</repo_root>
</project_identity>

<runtime_state>
<user_init_state>ready</user_init_state>
<bootstrap_token>DALEK-BOOT-a3f7</bootstrap_token>
</runtime_state>

<definition>
本文档是 Dalek 的工作空间——描述当前管理的项目的基础面。

内容边界：
  写什么：项目基础面——身份、技术栈、代码结构、构建方式、产品模型、架构、约定、当前状态
  不写什么：ticket 级事务细节、代码片段、频繁变化的运行时数据
  细节下沉：超出基础面的内容放 .dalek/control/knowledge/

约束：
  本文档注入到每次对话 context——体积直接影响 token 成本和注意力，必须精简
  与 kernel 冲突时 kernel 优先（kernel 是不可变法则，这里是项目约束）

更新触发：
  仅在项目基础面变化时更新——技术栈、架构、构建/部署方式、约定、阶段转换

初始化规则：
  user_init_state 为 uninitialized 时，必须先执行 .dalek/control/skills/project-init/ 完成初始化
</definition>

<project_overview>
这是 AgentSeal 的 dalek 工作区，当前主要承载调研、设计和 PM 计划文档。项目目标是做一个极简的 AI Agent 身份签名验证工具，用 Ed25519 密钥、公开身份文档与社交锚点完成身份声明、签名验证与信任缓存。
</project_overview>

<tech_stack>
当前仓库尚无产品实现代码，现阶段技术信息以设计为主：
- 文档：Markdown
- 计划中的产品形态：CLI 工具
- 计划中的核心密码学：Ed25519
- 计划中的公开身份载体：agentseal.json + GitHub/DNS/个人网站锚点
</tech_stack>

<structure>
顶层结构精简，当前以 dalek 元数据和讨论材料为主：
- temp.md：历史讨论导出
- .dalek/agent-kernel.md：Dalek 内核与操作约束
- .dalek/agent-user.md：项目用户态基础面
- .dalek/pm/plan.md：PM 阶段计划
- .dalek/control/knowledge/：调研与设计文档
- .dalek/bootstrap.sh：worktree bootstrap 占位脚本
</structure>

<product_model>
产品定位为面向开发者和 AI Agent 使用者的极简 CLI。核心闭环是：生成密钥对 -> 发布 agentseal.json 身份文档与锚点 -> 对消息签名 -> 对外验证签名与身份声明 -> 本地缓存信任对象。
</product_model>

<architecture>
当前仓库尚未进入实现阶段，架构以概念设计为主：
- 身份根：本地 Ed25519 密钥对
- 公开声明层：agentseal.json 身份文档
- 运行时验证层：签名、验签、锚点校验、known-seals 信任缓存
- 交互模型：以“名片 URL”作为握手入口，双方交换 URL 完成拉取与验证
</architecture>

<build_and_run>
当前没有可执行产品代码，也没有发现 Makefile、package.json、go.mod 等构建清单。现阶段主要工作方式是维护 Markdown 调研/设计文档；.dalek/bootstrap.sh 保持默认幂等占位脚本，可通过 `bash -n .dalek/bootstrap.sh` 校验语法。
</build_and_run>

<conventions>
当前以 PM/知识文档仓库方式运作：
- PM 不直接修改产品实现文件，产品开发需通过 ticket/worker 流程完成
- 文档与计划使用 Markdown
- 项目强调极简工具定位，避免过早标准化与复杂协议设计
- 初始化内容必须基于仓库现有证据填写，缺失实现细节不编造
</conventions>

<environment>
当前未发现明确的运行时依赖、版本约束或环境变量要求。仓库主要依赖 dalek 工作流与本地 shell 环境；产品实现阶段的语言运行时与依赖管理方案尚未落地。
</environment>

<current_state>
项目处于 phase-0 完成、phase-1 待启动状态。已有 OpenClaw 生态、身份层方案、Moltbook 案例和 AgentSeal 设计等文档，下一步是确定实际技术栈、搭建 CLI 项目骨架，并开始 MVP 实现。
</current_state>
