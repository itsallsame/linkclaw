package runtime

import "testing"

func TestNormalizeMessageStatus(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "pending", input: "pending", expect: MessageStatusPending},
		{name: "recoverable alias", input: "recoverable", expect: MessageStatusQueued},
		{name: "deferred alias", input: "deferred", expect: MessageStatusQueued},
		{name: "syncing alias", input: "syncing", expect: MessageStatusRecovering},
		{name: "received alias", input: "received", expect: MessageStatusRecovered},
		{name: "trim and lower", input: " Delivered ", expect: MessageStatusDelivered},
		{name: "empty", input: "   ", expect: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeMessageStatus(tc.input); got != tc.expect {
				t.Fatalf("NormalizeMessageStatus(%q) = %q, want %q", tc.input, got, tc.expect)
			}
		})
	}
}

func TestCanTransitionMessageStatus(t *testing.T) {
	tests := []struct {
		name   string
		from   string
		to     string
		expect bool
	}{
		{name: "pending to queued", from: MessageStatusPending, to: MessageStatusQueued, expect: true},
		{name: "queued to recovering", from: MessageStatusQueued, to: MessageStatusRecovering, expect: true},
		{name: "recovering back to queued", from: MessageStatusRecovering, to: MessageStatusQueued, expect: true},
		{name: "recovered to delivered", from: MessageStatusRecovered, to: MessageStatusDelivered, expect: true},
		{name: "recovered to queued blocked", from: MessageStatusRecovered, to: MessageStatusQueued, expect: false},
		{name: "delivered to queued blocked", from: MessageStatusDelivered, to: MessageStatusQueued, expect: false},
		{name: "failed to queued retry", from: MessageStatusFailed, to: MessageStatusQueued, expect: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanTransitionMessageStatus(tc.from, tc.to); got != tc.expect {
				t.Fatalf("CanTransitionMessageStatus(%q,%q) = %t, want %t", tc.from, tc.to, got, tc.expect)
			}
		})
	}
}

func TestMergeMessageStatus(t *testing.T) {
	tests := []struct {
		name   string
		from   string
		to     string
		expect string
	}{
		{name: "forward transition", from: MessageStatusPending, to: MessageStatusQueued, expect: MessageStatusQueued},
		{name: "alias transition", from: MessageStatusQueued, to: "syncing", expect: MessageStatusRecovering},
		{name: "invalid rollback", from: MessageStatusDelivered, to: MessageStatusQueued, expect: MessageStatusDelivered},
		{name: "empty source", from: "", to: MessageStatusRecovered, expect: MessageStatusRecovered},
		{name: "empty target keeps source", from: MessageStatusRecovered, to: "", expect: MessageStatusRecovered},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MergeMessageStatus(tc.from, tc.to); got != tc.expect {
				t.Fatalf("MergeMessageStatus(%q,%q) = %q, want %q", tc.from, tc.to, got, tc.expect)
			}
		})
	}
}
