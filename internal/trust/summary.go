package trust

import (
	"sort"
	"strings"
)

type TrustSummary struct {
	CanonicalID       string   `json:"canonical_id"`
	TrustLevel        string   `json:"trust_level"`
	VerificationState string   `json:"verification_state,omitempty"`
	ConfidenceScore   float64  `json:"confidence_score"`
	ConfidenceLevel   string   `json:"confidence_level"`
	Reachability      string   `json:"reachability"`
	RouteTypes        []string `json:"route_types,omitempty"`
	RiskFlags         []string `json:"risk_flags,omitempty"`
	Source            string   `json:"source,omitempty"`
	AsOf              string   `json:"as_of,omitempty"`
	Status            string   `json:"status"`
}

func BuildTrustSummary(profile TrustProfile) TrustSummary {
	trustLevel := strings.TrimSpace(profile.TrustLevel)
	if trustLevel == "" {
		trustLevel = "unknown"
	}

	verification := strings.TrimSpace(profile.VerificationState)
	source := strings.TrimSpace(profile.Source)
	routeTypes := normalizeStringList(profile.Discovery.RouteTypes)
	riskFlags := normalizeStringList(profile.RiskFlags)

	reachability := "unknown"
	if strings.TrimSpace(profile.Discovery.CanonicalID) != "" || len(routeTypes) > 0 || profile.Discovery.Source != "" {
		if profile.Discovery.Reachable {
			reachability = "reachable"
		} else {
			reachability = "unreachable"
		}
	}

	asOf := firstNonEmpty(
		strings.TrimSpace(profile.DecidedAt),
		strings.TrimSpace(profile.UpdatedAt),
		strings.TrimSpace(profile.Discovery.ResolvedAt),
		strings.TrimSpace(profile.CreatedAt),
	)

	statusParts := []string{
		trustLevel,
		strings.TrimSpace(profile.Confidence.Level),
		reachability,
	}
	status := strings.Join(statusParts, "|")

	// Guarantee deterministic order even if caller mutates profile slices.
	sort.Strings(routeTypes)
	sort.Strings(riskFlags)

	return TrustSummary{
		CanonicalID:       strings.TrimSpace(profile.CanonicalID),
		TrustLevel:        trustLevel,
		VerificationState: verification,
		ConfidenceScore:   profile.Confidence.Score,
		ConfidenceLevel:   strings.TrimSpace(profile.Confidence.Level),
		Reachability:      reachability,
		RouteTypes:        routeTypes,
		RiskFlags:         riskFlags,
		Source:            source,
		AsOf:              asOf,
		Status:            status,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
