package discovery

import "strings"

type SourceRanking map[string]int

func DefaultSourceRanking() SourceRanking {
	return SourceRanking{
		"refresh":         100,
		"import":          90,
		"libp2p-announce": 85,
		"libp2p":          80,
		"dht-announce":    75,
		"dht":             70,
		"nostr":           60,
		"manual":          55,
		"cache":           40,
		"unknown":         10,
	}
}

func (r SourceRanking) Rank(source string) int {
	normalized := normalizeSource(source)
	if normalized == "" {
		normalized = "unknown"
	}
	if rank, ok := r[normalized]; ok {
		return rank
	}
	return 40
}

func normalizeSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	switch source {
	case "", "none":
		return "unknown"
	case "refresh-peer", "known-refresh":
		return "refresh"
	case "known-import":
		return "import"
	default:
		return source
	}
}
