package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func New(prefix string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random id: %w", err)
	}
	if prefix == "" {
		return hex.EncodeToString(buf), nil
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(buf)), nil
}
