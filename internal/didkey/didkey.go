package didkey

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"math/big"
)

var base58Alphabet = []byte("123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz")

func FromBase64PublicKey(encoded string) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode public key: %w", err)
	}
	return FromEd25519PublicKey(raw)
}

func FromEd25519PublicKey(raw []byte) (string, error) {
	if len(raw) != ed25519.PublicKeySize {
		return "", fmt.Errorf("ed25519 public key must be %d bytes", ed25519.PublicKeySize)
	}
	prefixed := append([]byte{0xed, 0x01}, raw...)
	return "did:key:z" + base58Encode(prefixed), nil
}

func base58Encode(input []byte) string {
	if len(input) == 0 {
		return ""
	}

	zeros := 0
	for zeros < len(input) && input[zeros] == 0 {
		zeros++
	}

	value := new(big.Int).SetBytes(input)
	base := big.NewInt(58)
	mod := new(big.Int)
	encoded := make([]byte, 0, len(input)*2)

	for value.Sign() > 0 {
		value.DivMod(value, base, mod)
		encoded = append(encoded, base58Alphabet[mod.Int64()])
	}

	for i := 0; i < zeros; i++ {
		encoded = append(encoded, base58Alphabet[0])
	}

	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}

	if len(encoded) == 0 {
		return string(base58Alphabet[0])
	}
	return string(encoded)
}
