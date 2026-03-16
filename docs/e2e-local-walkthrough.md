# LinkClaw 本地端到端闭环验证

本文档用于在本地验证 LinkClaw phase-1 全部能力。不需要真实域名或外部基础设施。

## 前置条件

- Go 1.25+
- 项目已 clone 到本地

## Step 0: 编译

```bash
cd /path/to/claw-identity
go build -o linkclaw ./cmd/linkclaw
```

验证：`./linkclaw help` 应输出命令列表。

## Step 1: Alice 初始化身份

```bash
export ALICE_HOME=$(mktemp -d)/alice
./linkclaw init \
  --home "$ALICE_HOME" \
  --canonical-id "did:web:localhost:alice" \
  --display-name "Alice Agent" \
  --non-interactive \
  --json
```

验证：
- 输出 JSON 中 `ok: true`
- `$ALICE_HOME/state.db` 存在
- `$ALICE_HOME/keys/` 下有密钥文件

## Step 2: Alice 发布公开工件

```bash
export ALICE_PUBLISH="$ALICE_HOME/publish"
./linkclaw publish \
  --home "$ALICE_HOME" \
  --origin http://localhost:9100 \
  --tier full \
  --output "$ALICE_PUBLISH" \
  --json
```

验证：
- `$ALICE_PUBLISH/.well-known/did.json` 存在
- `$ALICE_PUBLISH/.well-known/webfinger` 存在
- `$ALICE_PUBLISH/.well-known/agent-card.json` 存在
- `$ALICE_PUBLISH/profile/index.html` 存在
- `$ALICE_PUBLISH/manifest.json` 存在且 `checks` 全部通过

## Step 3: 启动 Alice 的 HTTP 服务

```bash
cd "$ALICE_PUBLISH"
python3 -m http.server 9100 &
ALICE_PID=$!
cd -
```

验证：`curl http://localhost:9100/.well-known/did.json` 返回 DID 文档。

## Step 4: Bob 初始化身份

```bash
export BOB_HOME=$(mktemp -d)/bob
./linkclaw init \
  --home "$BOB_HOME" \
  --canonical-id "did:web:localhost:bob" \
  --display-name "Bob Agent" \
  --non-interactive \
  --json
```

## Step 5: Bob 探测 Alice

```bash
# inspect 是无状态操作，不需要 --home
./linkclaw inspect --json http://localhost:9100
```

验证：
- `verification_state` 应为 `consistent`（四个工件齐全且一致）
- `can_import` 应为 `true`
- `canonical_id` 应为 `did:web:localhost:alice`
- `artifacts` 数组应包含 4 个条目

## Step 6: Bob 导入 Alice

```bash
./linkclaw import \
  --home "$BOB_HOME" \
  http://localhost:9100 \
  --json
```

验证：
- `ok: true`
- Bob 的 `state.db` 中应有 Alice 的 contact 记录

## Step 7: Bob 管理信任关系

### 7a. 查看联系人列表

```bash
./linkclaw known ls --home "$BOB_HOME" --json
```

验证：列表中应包含 Alice 的 contact。

### 7b. 查看 Alice 详情

```bash
./linkclaw known show --home "$BOB_HOME" localhost:9100 --json
```

验证：应返回 Alice 的 canonical_id、handles、artifacts 等详情。

### 7c. 标记信任等级

```bash
./linkclaw known trust \
  --home "$BOB_HOME" \
  localhost:9100 \
  --level trusted \
  --reason "Verified via local e2e test" \
  --json
```

验证：`ok: true`，trust_level 更新为 `trusted`。

### 7d. 添加备注

```bash
./linkclaw known note \
  --home "$BOB_HOME" \
  localhost:9100 \
  --body "First contact via local walkthrough" \
  --json
```

验证：`ok: true`，note 已持久化。

### 7e. 刷新 Alice 的公开工件

```bash
./linkclaw known refresh \
  --home "$BOB_HOME" \
  localhost:9100 \
  --json
```

验证：`ok: true`，artifact snapshots 应更新。

### 7f. 再次查看详情确认变更

```bash
./linkclaw known show --home "$BOB_HOME" localhost:9100 --json
```

验证：trust_level 为 `trusted`，notes 和 events 历史应可见。

## Step 8: Index 抓取与搜索

```bash
# 抓取 Alice
./linkclaw index crawl \
  --home "$BOB_HOME" \
  http://localhost:9100 \
  --json

# 搜索
./linkclaw index search \
  --home "$BOB_HOME" \
  alice \
  --json
```

验证：
- crawl 应成功建立索引记录
- search 应返回包含 Alice 的结果，且可回链到源 URL

## Step 9: 清理

```bash
kill $ALICE_PID 2>/dev/null
rm -rf "$ALICE_HOME" "$BOB_HOME"
```

## 验证总结

跑完以上步骤，以下 phase-1 能力经过真实数据验证：

| 能力 | 覆盖步骤 |
| --- | --- |
| init（交互/非交互） | Step 1, 4 |
| publish（三档 bundle） | Step 2 |
| inspect（四态判定） | Step 5 |
| import（状态门禁） | Step 6 |
| known ls/show | Step 7a, 7b, 7f |
| known trust | Step 7c |
| known note | Step 7d |
| known refresh | Step 7e |
| index crawl/search | Step 8 |
| JSON envelope 合规 | 所有步骤均使用 --json |
