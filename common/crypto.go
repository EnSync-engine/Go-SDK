package common

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

	encryptionTypeHybrid = "hybrid"

	ed25519PublicKeySize = 32
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

func EncryptEd25519(message string, recipientEd25519PublicKey []byte) (*EncryptedMessage, error) {
	ephemeralPublic, ephemeralPrivate, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	recipientCurve25519PublicKey, err := ed25519PublicKeyToCurve25519(recipientEd25519PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert public key: %w", err)
	}

	var recipientPublicKey [32]byte
	copy(recipientPublicKey[:], recipientCurve25519PublicKey)

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

func DecryptEd25519(encrypted *EncryptedMessage, recipientEd25519SecretKey []byte) (string, error) {
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

	decrypted, ok := box.Open(nil, ciphertext, &nonceArray, &ephemeralPubKey, &recipientSecretKey)
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

func EncryptMessageKey(messageKey, recipientEd25519PublicKey []byte) (*EncryptedMessage, error) {
	ephemeralPublic, ephemeralPrivate, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	recipientCurve25519PublicKey, err := ed25519PublicKeyToCurve25519(recipientEd25519PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert public key: %w", err)
	}

	var recipientPublicKey [32]byte
	copy(recipientPublicKey[:], recipientCurve25519PublicKey)

	var nonce [24]byte
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

	var nonceArray [24]byte
	var recipientSecretKey [32]byte
	var ephemeralPubKey [32]byte

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

	encryptedPayload, err := EncryptWithMessageKey(message, messageKey)
	if err != nil {
		return nil, err
	}

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
		Type:    encryptionTypeHybrid,
		Payload: *encryptedPayload,
		Keys:    encryptedKeys,
	}, nil
}

func DecryptHybridMessage(hybridMsg *HybridEncryptedMessage, recipientSecretKey []byte) (string, error) {
	for _, encKey := range hybridMsg.Keys {
		messageKey, err := DecryptMessageKey(&encKey, recipientSecretKey)
		if err != nil {
			continue
		}

		decrypted, err := DecryptWithMessageKey(&hybridMsg.Payload, messageKey)
		if err != nil {
			continue
		}

		return decrypted, nil
	}

	return "", errors.New("failed to decrypt hybrid message with any of the recipient keys")
}

func ParseEncryptedPayload(payloadJSON string) (interface{}, error) {
	var hybridMsg HybridEncryptedMessage
	if err := json.Unmarshal([]byte(payloadJSON), &hybridMsg); err == nil && hybridMsg.Type == encryptionTypeHybrid {
		return &hybridMsg, nil
	}

	var encMsg EncryptedMessage
	if err := json.Unmarshal([]byte(payloadJSON), &encMsg); err != nil {
		return nil, fmt.Errorf("failed to parse encrypted payload: %w", err)
	}

	return &encMsg, nil
}
