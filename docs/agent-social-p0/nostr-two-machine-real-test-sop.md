# Nostr 双机真机测试 SOP

## 目标

验证以下真实链路已经成立：

- 两台真实机器安装新的 `linkclaw` OpenClaw 插件
- 使用插件内置的 `bundled-runtime/linkclaw`
- 通过第三方公共 Nostr relay 完成：
  - `send`
  - `sync`
  - `recover`

本 SOP 验证的是：

- `A -> third-party nostr relay -> B sync -> recovered`

## 测试环境

### 机器

- A 机器
  - IP: `43.165.180.204`
  - OpenClaw 源码路径: `/home/ubuntu/openclaw`
- B 机器
  - IP: `43.167.214.240`
  - OpenClaw 源码路径: `/home/ubuntu/openclaw`
  - A 机器可免密 SSH 到 B 机器

### 插件包

- 本次安装包：
  - `openclaw-plugin/linkclaw-0.1.0.tgz`

### 公共 relay

- `wss://relay.damus.io`
- `wss://nos.lol`

## 前置条件

1. 两台机器都已经安装 OpenClaw。
2. A 机器可以免密 SSH 到 B 机器。
3. 两台机器都安装了 `python3`。
4. A 机器当前仓库是最新实现，能重新打包插件。

## 步骤 1：重新打插件包

在 A 机器的 LinkClaw 仓库下执行：

```bash
cd /home/ubuntu/fork_lk/linkclaw/openclaw-plugin
npm run pack:plugin:tgz
```

预期结果：

- 生成新的 `linkclaw-0.1.0.tgz`
- 打包阶段自动重新编译并内置新的 `bundled-runtime/linkclaw`

## 步骤 2：把插件包放到两台机器

在 A 机器执行：

```bash
cp /home/ubuntu/fork_lk/linkclaw/openclaw-plugin/linkclaw-0.1.0.tgz /tmp/linkclaw-0.1.0.tgz
scp /tmp/linkclaw-0.1.0.tgz ubuntu@43.167.214.240:/tmp/linkclaw-0.1.0.tgz
```

预期结果：

- A 机器有 `/tmp/linkclaw-0.1.0.tgz`
- B 机器有 `/tmp/linkclaw-0.1.0.tgz`

## 步骤 3：卸载旧插件并安装新插件

### A 机器

```bash
cd /home/ubuntu/openclaw
node ./openclaw.mjs plugins uninstall linkclaw --force || true
node ./openclaw.mjs plugins install /tmp/linkclaw-0.1.0.tgz
```

### B 机器

```bash
ssh ubuntu@43.167.214.240 '
  cd /home/ubuntu/openclaw &&
  node ./openclaw.mjs plugins uninstall linkclaw --force || true &&
  node ./openclaw.mjs plugins install /tmp/linkclaw-0.1.0.tgz
'
```

预期结果：

- 两台机器都显示 `Installed plugin: linkclaw`
- 插件路径为：
  - A: `~/.openclaw/extensions/linkclaw`
  - B: `/home/ubuntu/.openclaw/extensions/linkclaw`

## 步骤 4：确认插件内置 runtime 可执行

### A 机器

```bash
~/.openclaw/extensions/linkclaw/bundled-runtime/linkclaw version
```

### B 机器

```bash
ssh ubuntu@43.167.214.240 '/home/ubuntu/.openclaw/extensions/linkclaw/bundled-runtime/linkclaw version'
```

预期结果：

- 两边都能成功输出 `linkclaw version`

## 步骤 5：创建双机测试 home

建议使用临时目录，避免污染已有环境。

### A 机器

```bash
export A_HOME=/tmp/linkclaw-real-a-$(date +%s)
~/.openclaw/extensions/linkclaw/bundled-runtime/linkclaw init \
  --home "$A_HOME" \
  --canonical-id did:web:43.165.180.204:alice \
  --display-name Alice \
  --non-interactive \
  --json

~/.openclaw/extensions/linkclaw/bundled-runtime/linkclaw message status \
  --home "$A_HOME" \
  --json
```

### B 机器

```bash
export B_HOME=/tmp/linkclaw-real-b-$(date +%s)
ssh ubuntu@43.167.214.240 '
  /home/ubuntu/.openclaw/extensions/linkclaw/bundled-runtime/linkclaw init \
    --home '"$B_HOME"' \
    --canonical-id did:web:43.167.214.240:bob \
    --display-name Bob \
    --non-interactive \
    --json &&
  /home/ubuntu/.openclaw/extensions/linkclaw/bundled-runtime/linkclaw message status \
    --home '"$B_HOME"' \
    --json
'
```

预期结果：

- 两边都成功初始化
- `state.db` 已生成
- `message status` 成功执行

## 步骤 6：为测试 home 写入 Nostr relay/binding 运行态

当前实现依赖运行态中的：

- `runtime_transport_bindings`
- `runtime_transport_relays`

建议直接用脚本向两边的 `state.db` 写入：

- self 的 `nostr_public_keys`
- `nostr_primary_public_key`
- `relay_urls`

本轮真实测试使用的是：

- `wss://relay.damus.io`
- `wss://nos.lol`

预期结果：

- A/B 两边都具备 Nostr transport binding
- A/B 两边都具备可读写 relay 配置

## 步骤 7：导出并交换 card

### A 导出 Alice card

```bash
~/.openclaw/extensions/linkclaw/bundled-runtime/linkclaw card export \
  --home "$A_HOME" \
  --json | jq -c '.result.card' > /tmp/real_a_card.json
```

### B 导出 Bob card

```bash
ssh ubuntu@43.167.214.240 '
  /home/ubuntu/.openclaw/extensions/linkclaw/bundled-runtime/linkclaw card export \
    --home '"$B_HOME"' \
    --json | jq -c '"'"'.result.card'"'"' > /tmp/real_b_card.json
'
```

### 交换 card 文件

```bash
scp ubuntu@43.167.214.240:/tmp/real_b_card.json /tmp/real_b_card.json
scp /tmp/real_a_card.json ubuntu@43.167.214.240:/tmp/real_a_card.json
```

### 双方导入

```bash
~/.openclaw/extensions/linkclaw/bundled-runtime/linkclaw card import \
  --home "$A_HOME" \
  --json \
  /tmp/real_b_card.json

ssh ubuntu@43.167.214.240 '
  /home/ubuntu/.openclaw/extensions/linkclaw/bundled-runtime/linkclaw card import \
    --home '"$B_HOME"' \
    --json \
    /tmp/real_a_card.json
'
```

预期结果：

- A 的 contacts 里出现 Bob
- B 的 contacts 里出现 Alice

## 步骤 8：A 向 B 发送消息

先取出 Bob 在 A 机器里的 `contact_id`，然后发送：

```bash
A_REF=$(python3 - <<'PY'
import sqlite3, os
db = os.path.join(os.environ["A_HOME"], "state.db")
con = sqlite3.connect(db)
row = con.execute(
    "select contact_id from contacts where canonical_id = ? limit 1",
    ("did:web:43.167.214.240:bob",),
).fetchone()
print(row[0] if row else "")
con.close()
PY
)

~/.openclaw/extensions/linkclaw/bundled-runtime/linkclaw message send \
  --home "$A_HOME" \
  --json \
  --body "hello from real machine A to B over nostr" \
  "$A_REF"
```

预期结果：

- `ok: true`
- `status: queued`
- `transport_status: deferred`
- `selected_route.Type: nostr`

## 步骤 9：B 执行 sync

```bash
for i in 1 2 3 4 5; do
  ssh ubuntu@43.167.214.240 '
    /home/ubuntu/.openclaw/extensions/linkclaw/bundled-runtime/linkclaw message sync \
      --home '"$B_HOME"' \
      --json
  '
  sleep 3
done
```

预期结果：

- 至少一轮出现：
  - `synced: 1`
  - `relay_calls: 2`

## 步骤 10：B 查看 thread

先取出 Alice 在 B 机器里的 `contact_id`，然后查看 thread：

```bash
B_REF=$(ssh ubuntu@43.167.214.240 "python3 - <<'PY'
import sqlite3
db = '$B_HOME/state.db'
con = sqlite3.connect(db)
row = con.execute(
    \"select contact_id from contacts where canonical_id = ? limit 1\",
    ('did:web:43.165.180.204:alice',),
).fetchone()
print(row[0] if row else '')
con.close()
PY")

ssh ubuntu@43.167.214.240 '
  /home/ubuntu/.openclaw/extensions/linkclaw/bundled-runtime/linkclaw message thread \
    --home '"$B_HOME"' \
    --json \
    '"$B_REF"'
'
```

预期结果：

- B 的 thread 中出现来自 Alice 的消息
- 该消息状态为：
  - `status: recovered`
  - `transport_status: recovered`

## 本次真实验证结果

本次双机测试的真实结果如下：

- A 发送成功
- A 发送状态：
  - `queued`
  - `deferred`
- B `sync` 成功：
  - `synced: 1`
  - `relay_calls: 2`
- B 在线程中看到了消息
- B 消息状态：
  - `recovered`
  - `transport_status: recovered`
- 命中的恢复路由：
  - `wss://nos.lol`

运行态观测：

- A `runtime_route_attempts = 2`
- B `runtime_messages = 1`

## 验收标准

本 SOP 验收通过的标准是：

1. 两台机器都成功安装新插件
2. 两台机器都能运行插件自带的 `bundled-runtime/linkclaw`
3. A 发送消息后状态为 `queued / deferred`
4. B 至少一次 `sync` 返回 `synced > 0`
5. B 的 `thread` 中出现消息
6. B 消息状态为 `recovered`

只要以上 6 条成立，就说明：

- 新插件包可用
- 插件内置 runtime 可用
- 真实双机环境下的 Nostr recoverable async messaging 已经跑通
