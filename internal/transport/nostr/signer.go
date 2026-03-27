package nostr

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

const AlgorithmSecp256k1Schnorr = "secp256k1-schnorr"

type EventSigner interface {
	PublicKey() string
	SignEventID(eventID string) (string, error)
}

type SchnorrSigner struct {
	privateKey *btcec.PrivateKey
	publicKey  string
}

func NewSchnorrSignerFromPrivateKeyFile(path string) (*SchnorrSigner, error) {
	data, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return nil, fmt.Errorf("read nostr private key file: %w", err)
	}
	return NewSchnorrSignerFromPrivateKeyHex(string(data))
}

func NewSchnorrSignerFromPrivateKeyHex(raw string) (*SchnorrSigner, error) {
	privateKeyBytes, err := decodeHex32(raw)
	if err != nil {
		return nil, fmt.Errorf("decode nostr private key: %w", err)
	}
	privateKey, _ := btcec.PrivKeyFromBytes(privateKeyBytes)
	publicKey := hex.EncodeToString(schnorr.SerializePubKey(privateKey.PubKey()))
	return &SchnorrSigner{
		privateKey: privateKey,
		publicKey:  publicKey,
	}, nil
}

func GenerateSchnorrPrivateKeyHex() (string, string, error) {
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		return "", "", fmt.Errorf("generate secp256k1 private key: %w", err)
	}
	privateKeyHex := hex.EncodeToString(privateKey.Serialize())
	publicKeyHex := hex.EncodeToString(schnorr.SerializePubKey(privateKey.PubKey()))
	return privateKeyHex, publicKeyHex, nil
}

func (s *SchnorrSigner) PublicKey() string {
	if s == nil {
		return ""
	}
	return s.publicKey
}

func (s *SchnorrSigner) SignEventID(eventID string) (string, error) {
	if s == nil || s.privateKey == nil {
		return "", fmt.Errorf("nostr signer private key is required")
	}
	eventHash, err := decodeHex32(eventID)
	if err != nil {
		return "", fmt.Errorf("decode event id: %w", err)
	}
	signature, err := schnorr.Sign(s.privateKey, eventHash)
	if err != nil {
		return "", fmt.Errorf("schnorr sign event id: %w", err)
	}
	return hex.EncodeToString(signature.Serialize()), nil
}

func decodeHex32(raw string) ([]byte, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if len(normalized) != 64 {
		return nil, fmt.Errorf("hex string must be 64 characters")
	}
	decoded, err := hex.DecodeString(normalized)
	if err != nil {
		return nil, err
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("hex string must decode to 32 bytes")
	}
	return decoded, nil
}
