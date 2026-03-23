package discovery

type SourceRanking map[string]int

func DefaultSourceRanking() SourceRanking {
	return SourceRanking{
		SourceRefresh:        100,
		SourceImport:         90,
		SourceLibp2pAnnounce: 85,
		SourceLibp2p:         80,
		SourceDHTAnnounce:    75,
		SourceDHT:            70,
		SourceNostr:          60,
		SourceManual:         55,
		SourceCache:          40,
		SourceUnknown:        10,
	}
}

func (r SourceRanking) Rank(source string) int {
	normalized := NormalizeSource(source)
	if rank, ok := r[normalized]; ok {
		return rank
	}
	if rank, ok := r[SourceUnknown]; ok {
		return rank
	}
	return 0
}
