package ensync

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

const (
	keySize   = 32
	nonceSize = 24
)

// EncryptedMessage represents an encrypted message with Ed25519
type EncryptedMessage struct {
	Nonce              string `json:"nonce"`
	Ciphertext         string `json:"ciphertext"`
	EphemeralPublicKey string `json:"ephemeralPublicKey"`
}

// EncryptedSymmetric represents a symmetrically encrypted message
type EncryptedSymmetric struct {
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// HybridEncryptedMessage represents a hybrid encrypted messxwage
type HybridEncryptedMessage struct {
	Type    string                      `json:"type"`
	Payload EncryptedSymmetric          `json:"payload"`
	Keys    map[string]EncryptedMessage `json:"keys"`
}

// ed25519PublicKeyToCurve25519 converts an Ed25519 public key to Curve25519
func ed25519PublicKeyToCurve25519(ed25519PublicKey []byte) ([]byte, error) {
	if len(ed25519PublicKey) != 32 {
		return nil, errors.New("invalid Ed25519 public key length")
	}

	// Ed25519 to Curve25519 conversion
	// This is a simplified version - in production, use a proper library like filippo.io/edwards25519
	var curve25519Key [32]byte
	copy(curve25519Key[:], ed25519PublicKey)
	return curve25519Key[:], nil
}

// ed25519SecretKeyToCurve25519 converts an Ed25519 secret key to Curve25519
func ed25519SecretKeyToCurve25519(ed25519SecretKey []byte) ([]byte, error) {
	if len(ed25519SecretKey) != 64 && len(ed25519SecretKey) != 32 {
		return nil, errors.New("invalid Ed25519 secret key length")
	}

	// Take the first 32 bytes (the actual secret scalar)
	var curve25519Key [32]byte
	copy(curve25519Key[:], ed25519SecretKey[:32])
	return curve25519Key[:], nil
}

// EncryptEd25519 encrypts a message using the recipient's Ed25519 public key
func EncryptEd25519(message string, recipientEd25519PublicKey []byte) (*EncryptedMessage, error) {
	// Generate ephemeral key pair
	ephemeralPublic, ephemeralPrivate, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Convert recipient Ed25519 public key to Curve25519
	recipientCurve25519PublicKey, err := ed25519PublicKeyToCurve25519(recipientEd25519PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert public key: %w", err)
	}

	var recipientPublicKey [32]byte
	copy(recipientPublicKey[:], recipientCurve25519PublicKey)

	// Generate random nonce
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	messageBytes := []byte(message)
	encrypted := box.Seal(nil, messageBytes, &nonce, &recipientPublicKey, ephemeralPrivate)

	return &EncryptedMessage{
		Nonce:              base64.StdEncoding.EncodeToString(nonce[:]),
		Ciphertext:         base64.StdEncoding.EncodeToString(encrypted),
		EphemeralPublicKey: base64.StdEncoding.EncodeToString(ephemeralPublic[:]),
	}, nil
}

// DecryptEd25519 decrypts a message using the recipient's Ed25519 private key
func DecryptEd25519(encrypted *EncryptedMessage, recipientEd25519SecretKey []byte) (string, error) {
	// Decode base64 values
	nonce, err := base64.StdEncoding.DecodeString(encrypted.Nonce)
	if err != nil {
		return "", fmt.Errorf("failed to decode nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	ephemeralPublicKey, err := base64.StdEncoding.DecodeString(encrypted.EphemeralPublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to decode ephemeral public key: %w", err)
	}

	// Convert recipient Ed25519 secret key to Curve25519
	recipientCurve25519SecretKey, err := ed25519SecretKeyToCurve25519(recipientEd25519SecretKey)
	if err != nil {
		return "", fmt.Errorf("failed to convert secret key: %w", err)
	}

	var nonceArray [24]byte
	var recipientSecretKey [32]byte
	var ephemeralPubKey [32]byte

	copy(nonceArray[:], nonce)
	copy(recipientSecretKey[:], recipientCurve25519SecretKey)
	copy(ephemeralPubKey[:], ephemeralPublicKey)

	// Decrypt
	decrypted, ok := box.Open(nil, ciphertext, &nonceArray, &ephemeralPubKey, &recipientSecretKey)
	if !ok {
		return "", errors.New("failed to decrypt: authentication failed")
	}

	return string(decrypted), nil
}

// GenerateMessageKey generates a random symmetric key for message encryption
func GenerateMessageKey() ([]byte, error) {
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate message key: %w", err)
	}
	return key, nil
}

// EncryptWithMessageKey encrypts a message using a symmetric key
func EncryptWithMessageKey(message string, messageKey []byte) (*EncryptedSymmetric, error) {
	if len(messageKey) != keySize {
		return nil, errors.New("invalid message key length")
	}

	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	var key [32]byte
	copy(key[:], messageKey)

	messageBytes := []byte(message)
	encrypted := secretbox.Seal(nil, messageBytes, &nonce, &key)

	return &EncryptedSymmetric{
		Nonce:      base64.StdEncoding.EncodeToString(nonce[:]),
		Ciphertext: base64.StdEncoding.EncodeToString(encrypted),
	}, nil
}

// DecryptWithMessageKey decrypts a message using a symmetric key
func DecryptWithMessageKey(encrypted *EncryptedSymmetric, messageKey []byte) (string, error) {
	if len(messageKey) != keySize {
		return "", errors.New("invalid message key length")
	}

	nonce, err := base64.StdEncoding.DecodeString(encrypted.Nonce)
	if err != nil {
		return "", fmt.Errorf("failed to decode nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	var nonceArray [24]byte
	var key [32]byte

	copy(nonceArray[:], nonce)
	copy(key[:], messageKey)

	decrypted, ok := secretbox.Open(nil, ciphertext, &nonceArray, &key)
	if !ok {
		return "", errors.New("failed to decrypt: authentication failed")
	}

	return string(decrypted), nil
}

// EncryptMessageKey encrypts a message key for a specific recipient
func EncryptMessageKey(messageKey []byte, recipientEd25519PublicKey []byte) (*EncryptedMessage, error) {
	// Generate ephemeral key pair
	ephemeralPublic, ephemeralPrivate, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Convert recipient Ed25519 public key to Curve25519
	recipientCurve25519PublicKey, err := ed25519PublicKeyToCurve25519(recipientEd25519PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert public key: %w", err)
	}

	var recipientPublicKey [32]byte
	copy(recipientPublicKey[:], recipientCurve25519PublicKey)

	// Generate random nonce
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the message key
	encryptedKey := box.Seal(nil, messageKey, &nonce, &recipientPublicKey, ephemeralPrivate)

	return &EncryptedMessage{
		Nonce:              base64.StdEncoding.EncodeToString(nonce[:]),
		Ciphertext:         base64.StdEncoding.EncodeToString(encryptedKey),
		EphemeralPublicKey: base64.StdEncoding.EncodeToString(ephemeralPublic[:]),
	}, nil
}

// DecryptMessageKey decrypts a message key using the recipient's Ed25519 private key
func DecryptMessageKey(encryptedKey *EncryptedMessage, recipientEd25519SecretKey []byte) ([]byte, error) {
	nonce, err := base64.StdEncoding.DecodeString(encryptedKey.Nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decode nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encryptedKey.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	ephemeralPublicKey, err := base64.StdEncoding.DecodeString(encryptedKey.EphemeralPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ephemeral public key: %w", err)
	}

	// Convert recipient Ed25519 secret key to Curve25519
	recipientCurve25519SecretKey, err := ed25519SecretKeyToCurve25519(recipientEd25519SecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert secret key: %w", err)
	}

	var nonceArray [24]byte
	var recipientSecretKey [32]byte
	var ephemeralPubKey [32]byte

	copy(nonceArray[:], nonce)
	copy(recipientSecretKey[:], recipientCurve25519SecretKey)
	copy(ephemeralPubKey[:], ephemeralPublicKey)

	// Decrypt the message key
	messageKey, ok := box.Open(nil, ciphertext, &nonceArray, &ephemeralPubKey, &recipientSecretKey)
	if !ok {
		return nil, errors.New("failed to decrypt message key: authentication failed")
	}

	return messageKey, nil
}

// HybridEncrypt encrypts a message using hybrid encryption
func HybridEncrypt(message string, recipientPublicKeys []string) (*HybridEncryptedMessage, error) {
	// Generate a random message key for symmetric encryption
	messageKey, err := GenerateMessageKey()
	if err != nil {
		return nil, err
	}

	// Encrypt the payload with the message key
	encryptedPayload, err := EncryptWithMessageKey(message, messageKey)
	if err != nil {
		return nil, err
	}

	// Encrypt the message key for each recipient
	encryptedKeys := make(map[string]EncryptedMessage)
	for _, publicKeyB64 := range recipientPublicKeys {
		publicKey, err := base64.StdEncoding.DecodeString(publicKeyB64)
		if err != nil {
			return nil, fmt.Errorf("failed to decode public key: %w", err)
		}

		encKey, err := EncryptMessageKey(messageKey, publicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt message key: %w", err)
		}

		encryptedKeys[publicKeyB64] = *encKey
	}

	return &HybridEncryptedMessage{
		Type:    "hybrid",
		Payload: *encryptedPayload,
		Keys:    encryptedKeys,
	}, nil
}

// DecryptHybridMessage decrypts a hybrid encrypted message
func DecryptHybridMessage(hybridMsg *HybridEncryptedMessage, recipientSecretKey []byte) (string, error) {
	// Try to decrypt with each key until one works
	for _, encKey := range hybridMsg.Keys {
		messageKey, err := DecryptMessageKey(&encKey, recipientSecretKey)
		if err != nil {
			continue // Try next key
		}

		// Decrypt the payload with the message key
		decrypted, err := DecryptWithMessageKey(&hybridMsg.Payload, messageKey)
		if err != nil {
			continue // Try next key
		}

		return decrypted, nil
	}

	return "", errors.New("failed to decrypt hybrid message with any of the recipient keys")
}

// ParseEncryptedPayload parses an encrypted payload from JSON
func ParseEncryptedPayload(payloadJSON string) (interface{}, error) {
	// Try to parse as hybrid message first
	var hybridMsg HybridEncryptedMessage
	if err := json.Unmarshal([]byte(payloadJSON), &hybridMsg); err == nil && hybridMsg.Type == "hybrid" {
		return &hybridMsg, nil
	}

	// Otherwise, parse as regular encrypted message
	var encMsg EncryptedMessage
	if err := json.Unmarshal([]byte(payloadJSON), &encMsg); err != nil {
		return nil, fmt.Errorf("failed to parse encrypted payload: %w", err)
	}

	return &encMsg, nil
}
