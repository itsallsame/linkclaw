package discovery

import (
	"strings"
	"time"
)

const (
	FreshnessStateUnknown = "unknown"
	FreshnessStateFresh   = "fresh"
	FreshnessStateStale   = "stale"
	FreshnessStateExpired = "expired"
)

type FreshnessPolicy struct {
	// FreshWindow defines how long a record is treated as fresh when fresh_until
	// is missing but resolved_at exists.
	FreshWindow time.Duration
	// StaleWindow defines the maximum age (from resolved_at) treated as stale.
	// Older records are expired.
	StaleWindow time.Duration
}

type Freshness struct {
	State            string `json:"state"`
	EvaluatedAt      string `json:"evaluated_at"`
	ResolvedAt       string `json:"resolved_at,omitempty"`
	FreshUntil       string `json:"fresh_until,omitempty"`
	AgeSeconds       int64  `json:"age_seconds,omitempty"`
	RemainingSeconds int64  `json:"remaining_seconds,omitempty"`
}

func DefaultFreshnessPolicy() FreshnessPolicy {
	return FreshnessPolicy{
		FreshWindow: 24 * time.Hour,
		StaleWindow: 72 * time.Hour,
	}
}

func EvaluateFreshness(now time.Time, resolvedAtRaw, freshUntilRaw string, policy FreshnessPolicy) Freshness {
	now = now.UTC()
	policy = normalizeFreshnessPolicy(policy)
	evaluatedAt := now.Format(time.RFC3339Nano)

	resolvedAt, hasResolvedAt := parseTimestamp(resolvedAtRaw)
	freshUntil, hasFreshUntil := parseTimestamp(freshUntilRaw)

	freshness := Freshness{
		State:       FreshnessStateUnknown,
		EvaluatedAt: evaluatedAt,
	}
	if hasResolvedAt {
		freshness.ResolvedAt = resolvedAt.Format(time.RFC3339Nano)
	}
	if hasFreshUntil {
		freshness.FreshUntil = freshUntil.Format(time.RFC3339Nano)
	}

	if hasFreshUntil {
		remaining := freshUntil.Sub(now)
		if remaining >= 0 {
			freshness.State = FreshnessStateFresh
			freshness.RemainingSeconds = int64(remaining / time.Second)
		} else {
			staleFor := now.Sub(freshUntil)
			if staleFor <= policy.FreshWindow {
				freshness.State = FreshnessStateStale
			} else {
				freshness.State = FreshnessStateExpired
			}
		}
		if hasResolvedAt && !now.Before(resolvedAt) {
			freshness.AgeSeconds = int64(now.Sub(resolvedAt) / time.Second)
		}
		return freshness
	}

	if hasResolvedAt {
		if now.Before(resolvedAt) {
			freshness.State = FreshnessStateFresh
			freshness.RemainingSeconds = int64(resolvedAt.Sub(now) / time.Second)
			return freshness
		}
		age := now.Sub(resolvedAt)
		freshness.AgeSeconds = int64(age / time.Second)
		switch {
		case age <= policy.FreshWindow:
			freshness.State = FreshnessStateFresh
			remaining := policy.FreshWindow - age
			if remaining > 0 {
				freshness.RemainingSeconds = int64(remaining / time.Second)
			}
		case age <= policy.StaleWindow:
			freshness.State = FreshnessStateStale
		default:
			freshness.State = FreshnessStateExpired
		}
		return freshness
	}

	return freshness
}

func normalizeFreshnessPolicy(policy FreshnessPolicy) FreshnessPolicy {
	defaults := DefaultFreshnessPolicy()
	if policy.FreshWindow <= 0 {
		policy.FreshWindow = defaults.FreshWindow
	}
	if policy.StaleWindow <= 0 {
		policy.StaleWindow = defaults.StaleWindow
	}
	if policy.StaleWindow < policy.FreshWindow {
		policy.StaleWindow = policy.FreshWindow
	}
	return policy
}

func freshnessWeight(state string) int {
	switch strings.TrimSpace(strings.ToLower(state)) {
	case FreshnessStateFresh:
		return 3
	case FreshnessStateStale:
		return 2
	case FreshnessStateExpired:
		return 1
	default:
		return 0
	}
}

func parseTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return ts.UTC(), true
}
