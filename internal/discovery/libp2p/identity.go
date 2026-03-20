package libp2p

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

type IdentityInput struct {
	CanonicalID         string
	SigningPublicKey    string
	EncryptionPublicKey string
}

type PeerIdentity struct {
	CanonicalID      string
	PeerID           string
	SignedPeerRecord string
}

type signedPeerRecord struct {
	CanonicalID         string `json:"canonical_id"`
	PeerID              string `json:"peer_id"`
	SigningPublicKey    string `json:"signing_public_key"`
	EncryptionPublicKey string `json:"encryption_public_key,omitempty"`
	Protocol            string `json:"protocol"`
}

func DerivePeerIdentity(input IdentityInput) (PeerIdentity, error) {
	canonicalID := strings.TrimSpace(input.CanonicalID)
	signingPublicKey := strings.TrimSpace(input.SigningPublicKey)
	if canonicalID == "" {
		return PeerIdentity{}, fmt.Errorf("canonical id is required")
	}
	if signingPublicKey == "" {
		return PeerIdentity{}, fmt.Errorf("signing public key is required")
	}

	digest := sha256.Sum256([]byte(canonicalID + "|" + signingPublicKey))
	peerID := "lcpeer:" + hex.EncodeToString(digest[:16])
	recordBytes, err := json.Marshal(signedPeerRecord{
		CanonicalID:         canonicalID,
		PeerID:              peerID,
		SigningPublicKey:    signingPublicKey,
		EncryptionPublicKey: strings.TrimSpace(input.EncryptionPublicKey),
		Protocol:            "libp2p-boundary-v1",
	})
	if err != nil {
		return PeerIdentity{}, fmt.Errorf("marshal signed peer record: %w", err)
	}

	return PeerIdentity{
		CanonicalID:      canonicalID,
		PeerID:           peerID,
		SignedPeerRecord: string(recordBytes),
	}, nil
}
