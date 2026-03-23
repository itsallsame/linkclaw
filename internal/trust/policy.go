package trust

import (
	"math"
	"strings"
)

const (
	ConfidenceLevelLow    = "low"
	ConfidenceLevelMedium = "medium"
	ConfidenceLevelHigh   = "high"
)

type PolicyInput struct {
	TrustLevel        string
	VerificationState string
	RiskFlags         []string
	Source            string
	HasDiscoveryData  bool
	Reachable         bool
	DiscoveryFresh    bool
	RouteTypes        []string
	HasSignedPeer     bool
}

type PolicyResult struct {
	Score   float64  `json:"score"`
	Level   string   `json:"level"`
	Factors []string `json:"factors,omitempty"`
}

type Policy struct {
	TrustLevelBase       map[string]float64
	VerificationAdjust   map[string]float64
	SourceAdjust         map[string]float64
	RiskPenalty          map[string]float64
	DefaultRiskPenalty   float64
	ReachableBonus       float64
	UnreachablePenalty   float64
	FreshBonus           float64
	StalePenalty         float64
	DirectRouteBonus     float64
	StoreForwardBonus    float64
	SignedPeerBonus      float64
	MismatchScoreCeiling float64
}

func DefaultPolicy() Policy {
	return Policy{
		TrustLevelBase: map[string]float64{
			"unknown":  0.35,
			"seen":     0.50,
			"verified": 0.70,
			"trusted":  0.85,
			"pinned":   0.95,
		},
		VerificationAdjust: map[string]float64{
			"discovered": -0.08,
			"resolved":   0.02,
			"consistent": 0.10,
			"mismatch":   -0.45,
		},
		SourceAdjust: map[string]float64{
			"import":      0.02,
			"refresh":     0.02,
			"known-trust": 0.08,
			"manual":      0.05,
		},
		RiskPenalty: map[string]float64{
			"manual":      0.01,
			"fixture":     0.01,
			"unverified":  0.12,
			"mismatch":    0.35,
			"spoofing":    0.40,
			"impostor":    0.35,
			"phishing":    0.35,
			"suspended":   0.30,
			"compromised": 0.45,
		},
		DefaultRiskPenalty:   0.05,
		ReachableBonus:       0.05,
		UnreachablePenalty:   0.04,
		FreshBonus:           0.04,
		StalePenalty:         0.06,
		DirectRouteBonus:     0.03,
		StoreForwardBonus:    0.01,
		SignedPeerBonus:      0.03,
		MismatchScoreCeiling: 0.35,
	}
}

func (p Policy) Evaluate(input PolicyInput) PolicyResult {
	trustLevel := strings.ToLower(strings.TrimSpace(input.TrustLevel))
	if trustLevel == "" {
		trustLevel = "unknown"
	}
	verification := strings.ToLower(strings.TrimSpace(input.VerificationState))
	source := strings.ToLower(strings.TrimSpace(input.Source))
	riskFlags := normalizeStringList(input.RiskFlags)
	routeTypes := normalizeStringList(input.RouteTypes)

	score := 0.0
	factors := make([]string, 0, 10+len(riskFlags))

	base, ok := p.TrustLevelBase[trustLevel]
	if !ok {
		base = p.TrustLevelBase["unknown"]
		factors = append(factors, "trust_level:unknown(fallback)")
	} else {
		factors = append(factors, "trust_level:"+trustLevel)
	}
	score += base

	if adjust, ok := p.VerificationAdjust[verification]; ok {
		score += adjust
		factors = append(factors, "verification_state:"+verification)
	}

	if adjust, ok := p.SourceAdjust[source]; ok {
		score += adjust
		factors = append(factors, "source:"+source)
	}

	if input.HasDiscoveryData {
		if input.Reachable {
			score += p.ReachableBonus
			factors = append(factors, "discovery:reachable")
		} else {
			score -= p.UnreachablePenalty
			factors = append(factors, "discovery:unreachable")
		}

		if input.DiscoveryFresh {
			score += p.FreshBonus
			factors = append(factors, "discovery:fresh")
		} else {
			score -= p.StalePenalty
			factors = append(factors, "discovery:stale")
		}

		if hasRouteType(routeTypes, "direct") {
			score += p.DirectRouteBonus
			factors = append(factors, "route:direct")
		}
		if hasRouteType(routeTypes, "store_forward") {
			score += p.StoreForwardBonus
			factors = append(factors, "route:store_forward")
		}
		if input.HasSignedPeer {
			score += p.SignedPeerBonus
			factors = append(factors, "signal:signed_peer_record")
		}
	} else {
		factors = append(factors, "discovery:missing")
	}

	for _, risk := range riskFlags {
		penalty := p.DefaultRiskPenalty
		if specific, ok := p.RiskPenalty[risk]; ok {
			penalty = specific
		}
		score -= penalty
		factors = append(factors, "risk:"+risk)
	}

	score = clamp(score, 0, 1)
	if verification == "mismatch" && score > p.MismatchScoreCeiling {
		score = p.MismatchScoreCeiling
		factors = append(factors, "verification_state:mismatch_ceiling")
	}
	score = roundTo(score, 4)

	return PolicyResult{
		Score:   score,
		Level:   confidenceLevel(score),
		Factors: factors,
	}
}

func confidenceLevel(score float64) string {
	switch {
	case score >= 0.80:
		return ConfidenceLevelHigh
	case score >= 0.55:
		return ConfidenceLevelMedium
	default:
		return ConfidenceLevelLow
	}
}

func hasRouteType(values []string, expected string) bool {
	expected = strings.TrimSpace(strings.ToLower(expected))
	if expected == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(strings.ToLower(value)) == expected {
			return true
		}
	}
	return false
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func roundTo(value float64, digits int) float64 {
	if digits <= 0 {
		return math.Round(value)
	}
	multiplier := math.Pow(10, float64(digits))
	return math.Round(value*multiplier) / multiplier
}
