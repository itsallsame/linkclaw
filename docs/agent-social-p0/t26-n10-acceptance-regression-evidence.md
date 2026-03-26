# T26 / N10 Acceptance And Regression Evidence

## Scope

This evidence set covers N10 for nostr recoverable async messaging on top of N3/N4/N7/N8/N9:

- direct success
- recoverable async success
- relay all failed
- partial relay success
- binding reject

## Truth Sources

- runtime send/recover orchestration: `internal/runtime/service.go`
- message runtime bridge and persistence: `internal/message/runtime_bridge.go`, `internal/message/service.go`
- CLI contract surfaces: `internal/cli/run_test.go`
- plugin command e2e surfaces: `openclaw-plugin/test/commands.test.ts`

## Evidence Matrix

| Scenario | Layer | Automated evidence | Key assertions |
| --- | --- | --- | --- |
| Direct success | Integration | `internal/message/service_test.go` -> `TestDirectDeliveryWhenBothHostsAreOnline` | outgoing message is `delivered`, transport is `direct`, receiver inbox has unread message |
| Recoverable async success | Unit | `internal/runtime/service_test.go` -> `TestServiceSendDirectFailureTriggersParallelNostrFanout` | direct failure triggers parallel nostr fanout, success path is normalized to `queued` recoverable async |
| Recoverable async success | Contract | `internal/cli/run_test.go` -> `TestRunMessageSendAndOutboxJSON`, `TestRunMessageSendHumanOutputUsesProductTerms` | user contract stays on product terms (`deferred`/recovery wording), no relay-internal leakage |
| Relay all failed | Unit | `internal/runtime/service_test.go` -> `TestServiceSendReturnsErrorWhenDirectNostrAndStoreForwardAllFail` | direct + nostr + store-forward all fail, runtime returns `no usable transport route`, outcomes recorded as failed |
| Relay all failed | Integration | `internal/message/service_test.go` -> `TestSendMarksMessageFailedWhenAllRuntimeRoutesFail` | runtime failure is persisted as message `failed` with failed transport status; nostr/store-forward failed attempts are recorded |
| Relay all failed | Contract | `internal/cli/run_test.go` -> `TestRunMessageSendJSONReportsFailedWhenAllRuntimeRoutesFail` | CLI JSON contract returns `ok=true` envelope with message `status=failed` and `transport_status=failed` |
| Partial relay success | Unit | `internal/runtime/service_test.go` -> `TestServiceSendDirectFailureTriggersParallelNostrFanout` | mixed nostr fanout (some fail, some accept) returns queued success on first accepted relay without falling back to store-forward |
| Binding reject | Integration | `internal/message/runtime_bridge_test.go` -> `TestSyncStoreForwardSkipsRecipientBindingMismatch` | recipient binding mismatch is rejected, message is not persisted, sync cursor still advances |
| Blocked path observability | E2E | `openclaw-plugin/test/commands.test.ts` -> `message connect-peer reports blocked readiness when no transport route is usable` | plugin-level flow exposes blocked readiness and reason in user-facing command result |

## Regression Commands

```bash
go test ./internal/runtime ./internal/message ./internal/cli
```

```bash
cd openclaw-plugin
npm test
```

## Conclusion

N10 target scenarios now have executable evidence across unit/integration/contract surfaces, with plugin e2e coverage retaining blocked-path observability and user-facing readiness semantics.
