package messagecrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
)

type EncryptedEnvelope struct {
	EphemeralPublicKey string
	Nonce              string
	Ciphertext         string
}

func GenerateX25519KeyPair() (publicKeyBase64, privateKeyBase64 string, err error) {
	curve := ecdh.X25519()
	privateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate x25519 key: %w", err)
	}
	return base64.RawStdEncoding.EncodeToString(privateKey.PublicKey().Bytes()),
		base64.RawStdEncoding.EncodeToString(privateKey.Bytes()),
		nil
}

func EncryptForRecipient(recipientPublicKeyBase64 string, plaintext []byte) (EncryptedEnvelope, error) {
	curve := ecdh.X25519()
	recipientPublicBytes, err := base64.RawStdEncoding.DecodeString(recipientPublicKeyBase64)
	if err != nil {
		return EncryptedEnvelope{}, fmt.Errorf("decode recipient encryption public key: %w", err)
	}
	recipientPublicKey, err := curve.NewPublicKey(recipientPublicBytes)
	if err != nil {
		return EncryptedEnvelope{}, fmt.Errorf("parse recipient encryption public key: %w", err)
	}
	ephemeralPrivateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return EncryptedEnvelope{}, fmt.Errorf("generate ephemeral x25519 key: %w", err)
	}
	sharedSecret, err := ephemeralPrivateKey.ECDH(recipientPublicKey)
	if err != nil {
		return EncryptedEnvelope{}, fmt.Errorf("derive shared secret: %w", err)
	}
	key := sha256.Sum256(sharedSecret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return EncryptedEnvelope{}, fmt.Errorf("create aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedEnvelope{}, fmt.Errorf("create aes-gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return EncryptedEnvelope{}, fmt.Errorf("generate aes-gcm nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return EncryptedEnvelope{
		EphemeralPublicKey: base64.RawStdEncoding.EncodeToString(ephemeralPrivateKey.PublicKey().Bytes()),
		Nonce:              base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext:         base64.RawStdEncoding.EncodeToString(ciphertext),
	}, nil
}

func DecryptWithPrivateKeyFile(privateKeyPath, ephemeralPublicKeyBase64, nonceBase64, ciphertextBase64 string) ([]byte, error) {
	privateBytes, err := readBase64File(privateKeyPath)
	if err != nil {
		return nil, err
	}
	curve := ecdh.X25519()
	privateKey, err := curve.NewPrivateKey(privateBytes)
	if err != nil {
		return nil, fmt.Errorf("parse x25519 private key: %w", err)
	}
	ephemeralPublicBytes, err := base64.RawStdEncoding.DecodeString(ephemeralPublicKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("decode ephemeral public key: %w", err)
	}
	ephemeralPublicKey, err := curve.NewPublicKey(ephemeralPublicBytes)
	if err != nil {
		return nil, fmt.Errorf("parse ephemeral public key: %w", err)
	}
	sharedSecret, err := privateKey.ECDH(ephemeralPublicKey)
	if err != nil {
		return nil, fmt.Errorf("derive shared secret: %w", err)
	}
	key := sha256.Sum256(sharedSecret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create aes-gcm: %w", err)
	}
	nonce, err := base64.RawStdEncoding.DecodeString(nonceBase64)
	if err != nil {
		return nil, fmt.Errorf("decode aes-gcm nonce: %w", err)
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt ciphertext: %w", err)
	}
	return plaintext, nil
}

func SignPayload(privateKeyPath string, payload []byte) (string, error) {
	privateKey, err := loadEd25519PrivateKey(privateKeyPath)
	if err != nil {
		return "", err
	}
	signature := ed25519.Sign(privateKey, payload)
	return base64.RawStdEncoding.EncodeToString(signature), nil
}

func VerifyPayload(publicKeyBase64 string, payload []byte, signatureBase64 string) error {
	publicKey, err := base64.RawStdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return fmt.Errorf("decode signing public key: %w", err)
	}
	signature, err := base64.RawStdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("signing public key has invalid size")
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), payload, signature) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func SaveBase64File(path string, valueBase64 string, perm os.FileMode) error {
	return os.WriteFile(path, []byte(valueBase64), perm)
}

func ReadBase64File(path string) ([]byte, error) {
	return readBase64File(path)
}

func readBase64File(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read base64 key file: %w", err)
	}
	decoded, err := base64.RawStdEncoding.DecodeString(string(raw))
	if err != nil {
		return nil, fmt.Errorf("decode base64 key file: %w", err)
	}
	return decoded, nil
}

func loadEd25519PrivateKey(path string) (ed25519.PrivateKey, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key file: %w", err)
	}
	block, _ := pem.Decode(bytes)
	if block == nil {
		return nil, fmt.Errorf("decode private key PEM: missing PEM block")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key PKCS8: %w", err)
	}
	privateKey, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not Ed25519")
	}
	return privateKey, nil
}
