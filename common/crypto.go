package common

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"filippo.io/edwards25519"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

const (
	keySize   = 32
	nonceSize = 24

	EncryptionTypeHybrid = "hybrid"

	ed25519PublicKeySize  = 32
	ed25519PrivateKeySize = 64
)

type EncryptedMessage struct {
	Nonce              string `json:"nonce"`
	Ciphertext         string `json:"ciphertext"`
	EphemeralPublicKey string `json:"ephemeralPublicKey"`
}

type EncryptedSymmetric struct {
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type HybridEncryptedMessage struct {
	Type    string                      `json:"type"`
	Payload EncryptedSymmetric          `json:"payload"`
	Keys    map[string]EncryptedMessage `json:"keys"`
}

func ed25519PublicKeyToCurve25519(ed25519PublicKey []byte) ([]byte, error) {
	if len(ed25519PublicKey) != ed25519PublicKeySize {
		return nil, errors.New("invalid Ed25519 public key length")
	}

	point, err := new(edwards25519.Point).SetBytes(ed25519PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid Ed25519 public key: %w", err)
	}

	return point.BytesMontgomery(), nil
}

func ed25519SecretKeyToCurve25519(ed25519SecretKey []byte) ([]byte, error) {
	if len(ed25519SecretKey) != ed25519PrivateKeySize && len(ed25519SecretKey) != keySize {
		return nil, errors.New("invalid Ed25519 secret key length")
	}

	// Use the standard RFC 7748 conversion method
	var seed [keySize]byte
	if len(ed25519SecretKey) == ed25519PrivateKeySize {
		// Extract seed from Ed25519 private key (first 32 bytes)
		copy(seed[:], ed25519SecretKey[:keySize])
	} else {
		// Already a 32-byte seed
		copy(seed[:], ed25519SecretKey)
	}

	// Hash the seed to get the scalar
	h := sha512.Sum512(seed[:])

	// Apply Curve25519 clamping as per RFC 7748
	h[0] &= 248  // Clear bottom 3 bits
	h[31] &= 127 // Clear top bit
	h[31] |= 64  // Set second-highest bit

	return h[:keySize], nil
}

func EncryptEd25519(payload string, publicKey []byte) (*EncryptedMessage, error) {
	if len(publicKey) != ed25519PublicKeySize {
		return nil, fmt.Errorf("invalid public key length: expected %d bytes, got %d", ed25519PublicKeySize, len(publicKey))
	}

	curve25519PublicKey, err := ed25519PublicKeyToCurve25519(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert Ed25519 public key to Curve25519: %w", err)
	}

	ephemeralPublicKey, ephemeralPrivateKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key pair: %w", err)
	}

	var nonce [nonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	var recipientPublicKey [keySize]byte
	copy(recipientPublicKey[:], curve25519PublicKey)

	ciphertext := box.Seal(nil, []byte(payload), &nonce, &recipientPublicKey, ephemeralPrivateKey)

	return &EncryptedMessage{
		Nonce:              base64.StdEncoding.EncodeToString(nonce[:]),
		Ciphertext:         base64.StdEncoding.EncodeToString(ciphertext),
		EphemeralPublicKey: base64.StdEncoding.EncodeToString(ephemeralPublicKey[:]),
	}, nil
}

func DecryptEd25519(encrypted *EncryptedMessage, secretKey []byte) (string, error) {
	nonce, err := base64.StdEncoding.DecodeString(encrypted.Nonce)
	if err != nil {
		return "", fmt.Errorf("failed to decode nonce: %w", err)
	}

	ephemeralPubKey, err := base64.StdEncoding.DecodeString(encrypted.EphemeralPublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to decode ephemeral public key: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	if len(nonce) != nonceSize {
		return "", fmt.Errorf("invalid nonce length: %d", len(nonce))
	}
	if len(ephemeralPubKey) != ed25519PublicKeySize {
		return "", fmt.Errorf("invalid ephemeral public key length: %d", len(ephemeralPubKey))
	}
	if len(secretKey) != ed25519PrivateKeySize {
		return "", fmt.Errorf("invalid secret key length: %d", len(secretKey))
	}

	curve25519SecretKey, err := ed25519SecretKeyToCurve25519(secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to convert secret key: %w", err)
	}

	var nonceArray [nonceSize]byte
	var ephemeralArray [keySize]byte
	var secretArray [keySize]byte

	copy(nonceArray[:], nonce)
	copy(ephemeralArray[:], ephemeralPubKey)
	copy(secretArray[:], curve25519SecretKey)

	decrypted, ok := box.Open(nil, ciphertext, &nonceArray, &ephemeralArray, &secretArray)
	if !ok {
		return "", errors.New("failed to decrypt: authentication failed")
	}

	return string(decrypted), nil
}

func GenerateMessageKey() ([]byte, error) {
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate message key: %w", err)
	}
	return key, nil
}

func encryptWithMessageKey(message string, messageKey []byte) (*EncryptedSymmetric, error) {
	if len(messageKey) != keySize {
		return nil, errors.New("invalid message key length")
	}

	var nonce [nonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	var key [keySize]byte
	copy(key[:], messageKey)

	messageBytes := []byte(message)
	encrypted := secretbox.Seal(nil, messageBytes, &nonce, &key)

	return &EncryptedSymmetric{
		Nonce:      base64.StdEncoding.EncodeToString(nonce[:]),
		Ciphertext: base64.StdEncoding.EncodeToString(encrypted),
	}, nil
}

func decryptWithMessageKey(encrypted *EncryptedSymmetric, messageKey []byte) (string, error) {
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

	var nonceArray [nonceSize]byte
	var key [keySize]byte

	copy(nonceArray[:], nonce)
	copy(key[:], messageKey)

	decrypted, ok := secretbox.Open(nil, ciphertext, &nonceArray, &key)
	if !ok {
		return "", errors.New("failed to decrypt: authentication failed")
	}

	return string(decrypted), nil
}

func encryptMessageKey(messageKey, recipientEd25519PublicKey []byte) (*EncryptedMessage, error) {
	ephemeralPublic, ephemeralPrivate, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	recipientCurve25519PublicKey, err := ed25519PublicKeyToCurve25519(recipientEd25519PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert public key: %w", err)
	}

	var recipientPublicKey [keySize]byte
	copy(recipientPublicKey[:], recipientCurve25519PublicKey)

	var nonce [nonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	encryptedKey := box.Seal(nil, messageKey, &nonce, &recipientPublicKey, ephemeralPrivate)

	return &EncryptedMessage{
		Nonce:              base64.StdEncoding.EncodeToString(nonce[:]),
		Ciphertext:         base64.StdEncoding.EncodeToString(encryptedKey),
		EphemeralPublicKey: base64.StdEncoding.EncodeToString(ephemeralPublic[:]),
	}, nil
}

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

	recipientCurve25519SecretKey, err := ed25519SecretKeyToCurve25519(recipientEd25519SecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert secret key: %w", err)
	}

	var nonceArray [nonceSize]byte
	var recipientSecretKey [keySize]byte
	var ephemeralPubKey [keySize]byte

	copy(nonceArray[:], nonce)
	copy(recipientSecretKey[:], recipientCurve25519SecretKey)
	copy(ephemeralPubKey[:], ephemeralPublicKey)

	messageKey, ok := box.Open(nil, ciphertext, &nonceArray, &ephemeralPubKey, &recipientSecretKey)
	if !ok {
		return nil, errors.New("failed to decrypt message key: authentication failed")
	}

	return messageKey, nil
}

func HybridEncrypt(message string, recipientPublicKeys []string) (*HybridEncryptedMessage, error) {
	messageKey, err := GenerateMessageKey()
	if err != nil {
		return nil, err
	}

	encryptedPayload, err := encryptWithMessageKey(message, messageKey)
	if err != nil {
		return nil, err
	}

	encryptedKeys := make(map[string]EncryptedMessage)
	for _, publicKeyB64 := range recipientPublicKeys {
		publicKey, err := base64.StdEncoding.DecodeString(publicKeyB64)
		if err != nil {
			return nil, fmt.Errorf("failed to decode public key: %w", err)
		}

		encKey, err := encryptMessageKey(messageKey, publicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt message key: %w", err)
		}

		encryptedKeys[publicKeyB64] = *encKey
	}

	return &HybridEncryptedMessage{
		Type:    EncryptionTypeHybrid,
		Payload: *encryptedPayload,
		Keys:    encryptedKeys,
	}, nil
}

func decryptHybridMessage(hybridMsg *HybridEncryptedMessage, recipientSecretKey []byte) (string, error) {
	for _, encKey := range hybridMsg.Keys {
		messageKey, err := DecryptMessageKey(&encKey, recipientSecretKey)
		if err != nil {
			continue
		}

		decrypted, err := decryptWithMessageKey(&hybridMsg.Payload, messageKey)
		if err != nil {
			continue
		}

		return decrypted, nil
	}

	return "", errors.New("failed to decrypt hybrid message with any of the recipient keys")
}

func parseEncryptedPayload(payloadJSON string) (interface{}, error) {
	var hybridMsg HybridEncryptedMessage
	if err := json.Unmarshal([]byte(payloadJSON), &hybridMsg); err == nil && hybridMsg.Type == EncryptionTypeHybrid {
		return &hybridMsg, nil
	}

	var encMsg EncryptedMessage
	if err := json.Unmarshal([]byte(payloadJSON), &encMsg); err != nil {
		return nil, fmt.Errorf("failed to parse encrypted payload: %w", err)
	}

	return &encMsg, nil
}

func generateEd25519KeyPair() (publicKey, privateKey []byte, err error) {
	return ed25519.GenerateKey(rand.Reader)
}

func ValidateEd25519KeyPair(publicKey, privateKey []byte) bool {
	if len(publicKey) != ed25519PublicKeySize || len(privateKey) != ed25519PrivateKeySize {
		return false
	}

	testMessage := []byte("key pair validation test")
	signature := ed25519.Sign(privateKey, testMessage)
	return ed25519.Verify(publicKey, testMessage, signature)
}
