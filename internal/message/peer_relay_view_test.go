package message

import (
	"slices"
	"testing"

	agentdiscovery "github.com/xiewanpeng/claw-identity/internal/discovery"
	"github.com/xiewanpeng/claw-identity/internal/transport"
)

func TestMergePeerRelayFactsHonorsPriority(t *testing.T) {
	view := mergePeerRelayFacts("did:key:z6MkPeer", []namedRelaySourceFact{
		{
			Source: relaySourceManualOverride,
			Fact: peerRelaySourceFact{
				RelayURLs: []string{"https://relay.manual.example"},
			},
		},
		{
			Source: relaySourceDiscovery,
			Fact: peerRelaySourceFact{
				RelayURLs:        []string{"https://relay.discovery.example", "wss://relay.runtime.nostr.example"},
				PublicKeys:       []string{"npub_discovery"},
				PrimaryPublicKey: "npub_discovery",
			},
		},
		{
			Source: relaySourceCardPublish,
			Fact: peerRelaySourceFact{
				RelayURLs:        []string{"wss://relay.card.nostr.example"},
				PublicKeys:       []string{"npub_card"},
				PrimaryPublicKey: "npub_card",
			},
		},
		{
			Source: relaySourceRegistry,
			Fact: peerRelaySourceFact{
				RelayURLs:        []string{"wss://relay.registry.nostr.example"},
				PublicKeys:       []string{"npub_registry"},
				PrimaryPublicKey: "npub_registry",
			},
		},
		{
			Source: relaySourceDefaultPublic,
			Fact: peerRelaySourceFact{
				RelayURLs:        []string{"wss://relay.default.nostr.example"},
				PublicKeys:       []string{"npub_default"},
				PrimaryPublicKey: "npub_default",
			},
		},
	})

	if got, want := view.EffectiveStoreForwardRelay, "https://relay.manual.example"; got != want {
		t.Fatalf("effective store-forward relay = %q, want %q", got, want)
	}
	if got, want := view.EffectiveStoreForwardSource, relaySourceManualOverride; got != want {
		t.Fatalf("effective store-forward source = %q, want %q", got, want)
	}
	if got, want := view.StoreForwardRelayURLs, []string{"https://relay.manual.example", "https://relay.discovery.example"}; !slices.Equal(got, want) {
		t.Fatalf("store-forward relays = %#v, want %#v", got, want)
	}
	if got, want := view.NostrRelayURLs, []string{
		"wss://relay.runtime.nostr.example",
		"wss://relay.card.nostr.example",
		"wss://relay.registry.nostr.example",
		"wss://relay.default.nostr.example",
	}; !slices.Equal(got, want) {
		t.Fatalf("nostr relays = %#v, want %#v", got, want)
	}
	if got, want := view.NostrPrimaryPublicKey, "npub_discovery"; got != want {
		t.Fatalf("nostr primary pubkey = %q, want %q", got, want)
	}
	if got, want := view.NostrPublicKeys, []string{"npub_discovery", "npub_card", "npub_registry", "npub_default"}; !slices.Equal(got, want) {
		t.Fatalf("nostr public keys = %#v, want %#v", got, want)
	}
}

func TestStoreForwardTargetsFromContactFiltersNostrRelayURLs(t *testing.T) {
	contact := contactRecord{
		RelayURL: "wss://relay.manual.nostr.example",
		StoreForwardHints: []string{
			"wss://relay.card.nostr.example",
			"https://relay.storeforward.example",
			"https://relay.storeforward.example",
		},
	}
	got := storeForwardTargetsFromContact(contact)
	want := []string{"https://relay.storeforward.example"}
	if !slices.Equal(got, want) {
		t.Fatalf("store-forward targets = %#v, want %#v", got, want)
	}
}

func TestApplyRelayViewToDiscoveryRecordAddsStoreForwardHintsAndRoutes(t *testing.T) {
	record := agentdiscovery.Record{
		CanonicalID:           "did:key:z6MkPeer",
		TransportCapabilities: []string{},
		RouteCandidates:       []transport.RouteCandidate{},
	}
	view := peerRelayPubKeyView{
		StoreForwardRelayURLs: []string{
			"https://relay.discovery.example",
			"wss://relay.discovery.nostr.example",
		},
	}

	updated := applyRelayViewToDiscoveryRecord(record, view)

	if got, want := updated.StoreForwardHints, []string{"https://relay.discovery.example"}; !slices.Equal(got, want) {
		t.Fatalf("store_forward_hints = %#v, want %#v", got, want)
	}
	if !slices.Contains(updated.TransportCapabilities, string(transport.RouteTypeStoreForward)) {
		t.Fatalf("transport capabilities = %#v, want store_forward", updated.TransportCapabilities)
	}

	foundRoute := false
	for _, route := range updated.RouteCandidates {
		if route.Type == transport.RouteTypeStoreForward && route.Target == "https://relay.discovery.example" {
			foundRoute = true
			break
		}
	}
	if !foundRoute {
		t.Fatalf("route candidates = %#v, want store_forward route to relay.discovery.example", updated.RouteCandidates)
	}
}
