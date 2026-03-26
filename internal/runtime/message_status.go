package runtime

import "strings"

const (
	MessageStatusPending    = "pending"
	MessageStatusQueued     = "queued"
	MessageStatusRecovering = "recovering"
	MessageStatusRecovered  = "recovered"
	MessageStatusDelivered  = "delivered"
	MessageStatusFailed     = "failed"
)

func NormalizeMessageStatus(status string) string {
	value := strings.ToLower(strings.TrimSpace(status))
	switch value {
	case "":
		return ""
	case "deferred", "recoverable":
		return MessageStatusQueued
	case "syncing", "recovering_async":
		return MessageStatusRecovering
	case "received", "synced":
		return MessageStatusRecovered
	default:
		return value
	}
}

func CanTransitionMessageStatus(from string, to string) bool {
	current := NormalizeMessageStatus(from)
	next := NormalizeMessageStatus(to)
	if current == "" || next == "" || current == next {
		return true
	}
	switch current {
	case MessageStatusPending:
		return next == MessageStatusQueued ||
			next == MessageStatusRecovering ||
			next == MessageStatusRecovered ||
			next == MessageStatusDelivered ||
			next == MessageStatusFailed
	case MessageStatusQueued:
		return next == MessageStatusRecovering ||
			next == MessageStatusRecovered ||
			next == MessageStatusDelivered ||
			next == MessageStatusFailed
	case MessageStatusRecovering:
		return next == MessageStatusQueued ||
			next == MessageStatusRecovered ||
			next == MessageStatusDelivered ||
			next == MessageStatusFailed
	case MessageStatusRecovered:
		return next == MessageStatusDelivered ||
			next == MessageStatusFailed
	case MessageStatusDelivered:
		return false
	case MessageStatusFailed:
		return next == MessageStatusQueued ||
			next == MessageStatusRecovering ||
			next == MessageStatusRecovered ||
			next == MessageStatusDelivered
	default:
		return true
	}
}

func MergeMessageStatus(current string, next string) string {
	from := NormalizeMessageStatus(current)
	to := NormalizeMessageStatus(next)
	if from == "" {
		return to
	}
	if to == "" {
		return from
	}
	if CanTransitionMessageStatus(from, to) {
		return to
	}
	return from
}
