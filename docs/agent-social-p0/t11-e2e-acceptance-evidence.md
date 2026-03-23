# T11 End-to-End Acceptance Evidence

## Scope

This evidence set targets the T11 acceptance goals:

- discover -> inspect -> connect -> message -> recover full-chain validation
- online send regression
- offline recovery regression
- blocked/unavailable-path observability

## Truth Sources

- runtime and CLI behavior: `internal/message`, `internal/runtime`, `internal/cli`
- OpenClaw plugin command behavior: `openclaw-plugin/src/commands.ts`
- regression tests in `internal/*_test.go` and `openclaw-plugin/test/*.test.ts`

## Evidence Matrix

| Acceptance item | Automated evidence | Key assertions |
| --- | --- | --- |
| Full chain: discover -> inspect -> connect -> message -> recover | `openclaw-plugin/test/commands.test.ts` -> `discover inspect connect message recover flow stays healthy end-to-end` | inspect is consistent/importable, discovery includes fixture record, card connect succeeds, message send succeeds, sync recovers one message, thread contains sent text |
| Online send works | `internal/message/service_test.go` -> `TestDirectDeliveryWhenBothHostsAreOnline` | message status is delivered, transport status is direct, receiver inbox gets unread message |
| Offline recovery works | `openclaw-plugin/test/commands.test.ts` -> `runStatusCommand reflects offline recovery state after sync`; `internal/cli/run_test.go` -> `TestRunMessageSyncJSONWithRelay` | sync reports recovered messages, status shows recovered counters and offline recovery readiness |
| Blocked state is observable | `openclaw-plugin/test/commands.test.ts` -> `message connect-peer reports blocked readiness when no transport route is usable`; `internal/runtime/service_test.go` -> `TestServiceConnectPeerReturnsUnconnectedWhenNoUsableRoute` | connect result has `connected=false` with a non-empty reason (`no usable transport route`) |

## Regression Commands

- Plugin regression suite:

```bash
cd openclaw-plugin
npm test
```

- Runtime/CLI regression subset:

```bash
go test ./internal/message ./internal/runtime ./internal/cli
```

## Conclusion

With the tests above passing, the T11 acceptance requirements are covered by executable evidence for chain integrity, online send, offline recovery, and blocked-state observability.
