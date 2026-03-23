package discovery

import "strings"

const (
	SourceRefresh        = "refresh"
	SourceImport         = "import"
	SourceLibp2pAnnounce = "libp2p-announce"
	SourceLibp2p         = "libp2p"
	SourceDHTAnnounce    = "dht-announce"
	SourceDHT            = "dht"
	SourceNostr          = "nostr"
	SourceManual         = "manual"
	SourceCache          = "cache"
	SourceUnknown        = "unknown"
)

var supportedDiscoverySources = map[string]struct{}{
	SourceRefresh:        {},
	SourceImport:         {},
	SourceLibp2pAnnounce: {},
	SourceLibp2p:         {},
	SourceDHTAnnounce:    {},
	SourceDHT:            {},
	SourceNostr:          {},
	SourceManual:         {},
	SourceCache:          {},
	SourceUnknown:        {},
}

func NormalizeSource(source string) string {
	value := strings.ToLower(strings.TrimSpace(source))
	switch value {
	case "", "none":
		return SourceUnknown
	case "refresh-peer", "known-refresh":
		return SourceRefresh
	case "known-import":
		return SourceImport
	case "stale-cache", "runtime-send", "runtime-cache":
		return SourceCache
	}
	if _, ok := supportedDiscoverySources[value]; ok {
		return value
	}
	return SourceUnknown
}

func IsSupportedSourceFilter(source string) bool {
	value := strings.ToLower(strings.TrimSpace(source))
	switch value {
	case "", "none",
		"refresh-peer", "known-refresh",
		"known-import",
		"stale-cache", "runtime-send", "runtime-cache":
		return true
	}
	_, ok := supportedDiscoverySources[value]
	return ok
}
