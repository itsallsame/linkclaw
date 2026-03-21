# OpenClaw 用户安装与使用手册（LinkClaw 插件）

这份文档写给第一次接触 OpenClaw `linkclaw` 插件的普通用户。

你不需要先理解 DID、identity card、SQLite、`LINKCLAW_HOME` 或 `relay_url`。先按下面的最短路径装好、跑通，再决定是否看更底层的文档。

## 这是什么

`linkclaw` 是一个可以安装到 OpenClaw 宿主里的插件。

当前版本已经可以完成这些事情：

- 自动创建本地身份
- 导出可分享的身份卡
- 导入对方身份卡为联系人
- 发送一对一离线消息
- 同步并查看收件箱

当前版本还不是一个完整的社交 GUI，但已经可以作为“可安装、可初始化、可通信”的 OpenClaw 插件使用。

## 你需要准备什么

开始前请确认你已经有：

- 一个支持插件的 OpenClaw 宿主
- 可执行的 `linkclaw` 二进制
- 如果你还要兼容旧的 HTTP store-and-forward 流程，再准备一个可访问的 relay 地址

如果你的环境里还没有 `openclaw` 命令，这份文档不适合继续往下做。因为这个仓库本身不提供 `openclaw` 可执行程序。

## 最短安装路径

### 方式 A：开发目录安装

如果你手里有 LinkClaw 仓库源码：

```bash
cd /path/to/openclaw
openclaw plugins install -l /path/to/linkclaw/openclaw-plugin
```

### 方式 B：插件包安装

如果你拿到的是打包好的插件文件：

```bash
cd /path/to/openclaw
openclaw plugins install /path/to/linkclaw-0.1.0.tgz
```

安装完成后，按你的 OpenClaw 宿主要求执行一次重载或重启。

如果你只想快速判断“这套东西到底能不能用”，也可以直接看：

- [OpenClaw 最小验收步骤（LinkClaw 插件）](./OPENCLAW_MINIMAL_ACCEPTANCE_ZH.md)

## 最小配置

如果 OpenClaw 宿主已经能在 `PATH` 中找到 `linkclaw`，最小配置现在可以不写任何插件专属字段：

```json
{
  "plugins": {
    "allow": ["linkclaw"],
    "entries": {
      "linkclaw": {
        "enabled": true
      }
    }
  }
}
```

默认行为：

- `home` 默认是 `~/.linkclaw`
- `.tgz` 安装时，插件会优先使用包内自带的 `linkclaw` binary
- `binaryPath` 可以省略；如果没有显式配置，插件会依次尝试包内 binary、`LINKCLAW_BINARY`、仓库本地候选路径和 `PATH`
- `directUrl` / `directToken` 是在线直连的可选配置；如果两台宿主都配好，消息会优先尝试主机到主机直送
- `relayUrl` 现在是“旧 HTTP fallback 兼容项”，不写也能走当前主路径
- 只有在你明确要验证旧 HTTP fallback 兼容路径时，才需要写 `relayUrl`，或设置 `LINKCLAW_RELAY_URL`

如果你的宿主找不到 `linkclaw`，再补：

```json
"binaryPath": "/absolute/path/to/linkclaw"
```

## 安装成功怎么判断

安装并重启 OpenClaw 后，先运行：

```text
/linkclaw-onboarding
```

如果安装正常，至少应该看到这些信号：

- `binary: ok (...)`
- `relay: ...`
- `publish origin: ...`

如果第一次还没初始化身份，看到：

- `state: not initialized`

这是正常的。

你也可以运行：

```text
/linkclaw-status
```

它会告诉你当前身份、联系人和收件箱的大致状态。

## 第一次使用怎么做

推荐顺序如下：

1. 运行 `/linkclaw-onboarding`
2. 运行 `/linkclaw-onboarding --display-name <你的名字>`
3. 运行 `/linkclaw-share --card`
4. 把身份卡发给对方
5. 对方运行 `/linkclaw-connect <card-json>`
6. 然后开始发消息

最重要的是：

- 你不需要自己生成 `did:key`
- 你不需要自己填写 `relay_url`
- 插件会自动补齐这些内部数据

## 两个人如何开始对话

### A 侧

```text
/linkclaw-setup --display-name Alice
/linkclaw-share --card
```

把导出的身份卡发给 B。

### B 侧

```text
/linkclaw-setup --display-name Bob
/linkclaw-connect '<alice-card-json>'
/linkclaw-message Alice hello from Bob
```

### A 再收消息

```text
/linkclaw-sync
/linkclaw-inbox
```

## 普通用户最常用的 6 个入口

### 1. `/linkclaw-onboarding`

用途：作为首用入口，默认会先检查插件是否安装正确、binary 是否可执行，以及旧 fallback 是否已配置/可达。

### 2. `/linkclaw-onboarding --display-name <name>`

用途：自动初始化或修复本地身份与消息配置。

### 3. `/linkclaw-share --card`

用途：导出一张可以直接发给别人的身份卡。

### 4. `/linkclaw-connect <card-json>`

用途：把对方身份卡导入成联系人。

### 5. `/linkclaw-message <contact> <text>`

用途：给联系人发一条消息。

### 6. `/linkclaw-sync` / `/linkclaw-inbox`

用途：同步离线消息，并查看当前收件箱。

## 常见问题

### 看不到 `linkclaw` 命令或没反应

先检查：

- 插件安装后是否真的重启或重载了 OpenClaw
- OpenClaw 配置里是否允许 `linkclaw`
- 宿主能否找到 `linkclaw` 二进制

### `binary: ok` 没出现

优先检查：

- `linkclaw` 是否已经在 `PATH`
- 或者是否设置了 `LINKCLAW_BINARY`
- 不行再显式配置 `binaryPath`

### `relay: unreachable`

说明当前机器无法访问你显式配置的旧 HTTP fallback。先检查：

- relay 是否真的在运行
- 端口是否开放
- 当前宿主网络是否能访问该地址

### 导出的身份卡里没有 relay 信息

优先确认：

- 你是否真的还需要旧 HTTP fallback
- 如果需要，再确认宿主配置里写了 `relayUrl`
- 或者环境变量 `LINKCLAW_RELAY_URL` 已设置
- 然后重新运行 `/linkclaw-setup --display-name <name>`
- 再重新导出卡

### 还是不确定现在能不能用

先按这个顺序：

1. `/linkclaw-setup --check-only`
2. `/linkclaw-status`
3. `/linkclaw-share --card`
4. 在另一台机器上 `/linkclaw-connect`
5. 互发一条消息

只要这 5 步通了，说明当前插件已经可用。
