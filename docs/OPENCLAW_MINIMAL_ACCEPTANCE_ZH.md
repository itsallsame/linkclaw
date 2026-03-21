# OpenClaw 最小验收步骤（LinkClaw 插件）

这份文档用于在一个干净的 OpenClaw 宿主上，快速判断 LinkClaw 插件是否已经达到“可安装、可初始化、可通信”的最低标准。

## 前提

开始前，请确认：

- OpenClaw 宿主已经可运行
- `linkclaw` 已经在 `PATH` 中，或已经设置 `LINKCLAW_BINARY`
- 如果你还要兼容旧的 HTTP store-and-forward fallback，再额外准备一个 relay

最小配置模板见：

- [OPENCLAW_MINIMAL_PLUGIN_CONFIG.json](./OPENCLAW_MINIMAL_PLUGIN_CONFIG.json)

## 验收步骤

### 1. 安装插件

开发目录安装：

```bash
openclaw plugins install -l /path/to/linkclaw/openclaw-plugin
```

或使用打包文件安装：

```bash
openclaw plugins install /path/to/linkclaw-0.1.0.tgz
```

安装完成后，按宿主要求重启或重载。

### 2. 写入最小配置

把下面这段配置合并到 OpenClaw 宿主配置中：

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

### 3. 运行首用检查

在 OpenClaw 中运行：

```text
/linkclaw-onboarding
```

通过标准：

- 能返回结果
- 能看到 `binary: ok (...)`
- 能看到 `relay: ...`
- 如果没有显式配置旧 fallback，看到 `relay: not configured` 也是正常的
- 第一次运行时看到 `state: not initialized` 是正常的

### 4. 初始化身份

继续运行：

```text
/linkclaw-onboarding --display-name Alice
```

通过标准：

- 身份初始化完成
- 自动生成本地身份
- 不要求用户填写 `canonical id`

### 5. 导出身份卡

运行：

```text
/linkclaw-share --card
```

通过标准：

- 返回一张 identity card
- 如果没有显式配置旧 fallback，card 中可以不包含 `relay_url`
- 如果你显式配置了 `relayUrl`，card 中应包含 `relay_url`

### 6. 另一台宿主导入并发消息

在第二台宿主重复步骤 1 到 4，然后：

```text
/linkclaw-connect '<alice-card-json>'
/linkclaw-message Alice hello from Bob
```

通过标准：

- 联系人导入成功
- 消息发送状态不是卡在本地未路由
- 如果你在验旧 HTTP fallback 兼容路径，再确认双方都显式配置了同一个 `relayUrl`

### 7. 第一台宿主同步并收消息

在第一台宿主运行：

```text
/linkclaw-sync
/linkclaw-inbox
```

通过标准：

- 至少同步到 1 条消息
- 收件箱里能看到来自 Bob 的会话

## 最终判断

只要上面 7 步都通过，就说明当前宿主上的 LinkClaw 插件已经满足最小交付标准：

- 可安装
- 可首用
- 可导卡
- 可导入联系人
- 可一对一通信
