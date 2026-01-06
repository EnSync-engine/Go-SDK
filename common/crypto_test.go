package common

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

const (
	testMessageHello = "Hello, World!"
	testMessage      = "Test message"
)

func TestEd25519PublicKeyToCurve25519(t *testing.T) {
	// Test with valid Ed25519 public key
	publicKey, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("Failed to generate Ed25519 key pair: %v", err)
	}

	curve25519Key, err := ed25519PublicKeyToCurve25519(publicKey)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if len(curve25519Key) != 32 {
		t.Errorf("Expected key length 32, got: %d", len(curve25519Key))
	}

	// Test with invalid key length
	invalidKey := make([]byte, 31)
	_, err = ed25519PublicKeyToCurve25519(invalidKey)
	if err == nil {
		t.Error("Expected error for invalid key length, got nil")
	}
	if !strings.Contains(err.Error(), "invalid Ed25519 public key length") {
		t.Errorf("Expected specific error message, got: %v", err)
	}

	// Test with nil key
	_, err = ed25519PublicKeyToCurve25519(nil)
	if err == nil {
		t.Error("Expected error for nil key, got nil")
	}
}

func TestEd25519SecretKeyToCurve25519(t *testing.T) {
	// Test with valid Ed25519 private key (64 bytes)
	_, privateKey, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("Failed to generate Ed25519 key pair: %v", err)
	}

	curve25519Key, err := ed25519SecretKeyToCurve25519(privateKey)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if len(curve25519Key) != 32 {
		t.Errorf("Expected key length 32, got: %d", len(curve25519Key))
	}

	// Test with 32-byte seed (first 32 bytes of Ed25519 private key)
	seed := privateKey[:32]
	curve25519Key, err = ed25519SecretKeyToCurve25519(seed)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if len(curve25519Key) != 32 {
		t.Errorf("Expected key length 32, got: %d", len(curve25519Key))
	}

	invalidKey := make([]byte, 31)
	_, err = ed25519SecretKeyToCurve25519(invalidKey)
	if err == nil {
		t.Error("Expected error for invalid key length, got nil")
	}
	if !strings.Contains(err.Error(), "invalid Ed25519 secret key length") {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func TestEncryptDecryptEd25519(t *testing.T) {
	// Generate proper Ed25519 key pair
	ed25519PublicKey, ed25519PrivateKey, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("Failed to generate Ed25519 key pair: %v", err)
	}

	message := testMessageHello

	// Encrypt the message
	encrypted, err := EncryptEd25519(message, ed25519PublicKey)
	if err != nil {
		t.Fatalf("Failed to encrypt message: %v", err)
	}

	// Verify encrypted message structure
	if encrypted.Nonce == "" {
		t.Error("Expected non-empty nonce")
	}
	if encrypted.Ciphertext == "" {
		t.Error("Expected non-empty ciphertext")
	}
	if encrypted.EphemeralPublicKey == "" {
		t.Error("Expected non-empty ephemeral public key")
	}

	// Verify base64 encoding
	_, err = base64.StdEncoding.DecodeString(encrypted.Nonce)
	if err != nil {
		t.Errorf("Invalid base64 nonce: %v", err)
	}
	_, err = base64.StdEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil {
		t.Errorf("Invalid base64 ciphertext: %v", err)
	}
	_, err = base64.StdEncoding.DecodeString(encrypted.EphemeralPublicKey)
	if err != nil {
		t.Errorf("Invalid base64 ephemeral public key: %v", err)
	}

	// Decrypt the message
	decrypted, err := DecryptEd25519(encrypted, ed25519PrivateKey)
	if err != nil {
		t.Fatalf("Failed to decrypt message: %v", err)
	}

	if decrypted != message {
		t.Errorf("Expected decrypted message %q, got %q", message, decrypted)
	}
}

func TestEncryptEd25519InvalidKey(t *testing.T) {
	message := testMessageHello
	invalidKey := make([]byte, 31) // Invalid length

	_, err := EncryptEd25519(message, invalidKey)
	if err == nil {
		t.Error("Expected error for invalid public key, got nil")
	}
	if !strings.Contains(err.Error(), "invalid public key length") {
		t.Errorf("Expected key length error, got: %v", err)
	}
}

func TestDecryptEd25519InvalidData(t *testing.T) {
	privateKey := make([]byte, 64)
	_, err := rand.Read(privateKey)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Test with invalid nonce
	invalidEncrypted := &EncryptedMessage{
		Nonce:              "invalid-base64",
		Ciphertext:         "dGVzdA==",
		EphemeralPublicKey: "dGVzdA==",
	}
	_, err = DecryptEd25519(invalidEncrypted, privateKey)
	if err == nil {
		t.Error("Expected error for invalid nonce, got nil")
	}

	// Test with invalid ciphertext
	invalidEncrypted = &EncryptedMessage{
		Nonce:              "dGVzdA==",
		Ciphertext:         "invalid-base64",
		EphemeralPublicKey: "dGVzdA==",
	}
	_, err = DecryptEd25519(invalidEncrypted, privateKey)
	if err == nil {
		t.Error("Expected error for invalid ciphertext, got nil")
	}

	// Test with invalid ephemeral public key
	invalidEncrypted = &EncryptedMessage{
		Nonce:              "dGVzdA==",
		Ciphertext:         "dGVzdA==",
		EphemeralPublicKey: "invalid-base64",
	}
	_, err = DecryptEd25519(invalidEncrypted, privateKey)
	if err == nil {
		t.Error("Expected error for invalid ephemeral public key, got nil")
	}
}

func TestGenerateMessageKey(t *testing.T) {
	key, err := GenerateMessageKey()
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if len(key) != keySize {
		t.Errorf("Expected key length %d, got: %d", keySize, len(key))
	}

	// Generate another key and ensure they're different
	key2, err := GenerateMessageKey()
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if bytes.Equal(key, key2) {
		t.Error("Expected different keys, got identical keys")
	}
}

func TestEncryptDecryptWithMessageKey(t *testing.T) {
	message := "Test message for symmetric encryption"

	// Generate a message key
	messageKey, err := GenerateMessageKey()
	if err != nil {
		t.Fatalf("Failed to generate message key: %v", err)
	}

	// Encrypt the message
	encrypted, err := EncryptWithMessageKey(message, messageKey)
	if err != nil {
		t.Fatalf("Failed to encrypt with message key: %v", err)
	}

	// Verify encrypted structure
	if encrypted.Nonce == "" {
		t.Error("Expected non-empty nonce")
	}
	if encrypted.Ciphertext == "" {
		t.Error("Expected non-empty ciphertext")
	}

	// Decrypt the message
	decrypted, err := DecryptWithMessageKey(encrypted, messageKey)
	if err != nil {
		t.Fatalf("Failed to decrypt with message key: %v", err)
	}

	if decrypted != message {
		t.Errorf("Expected decrypted message %q, got %q", message, decrypted)
	}
}

func TestEncryptWithMessageKeyInvalidKey(t *testing.T) {
	message := testMessage

	// Test with invalid key length
	invalidKey := make([]byte, 31)
	_, err := EncryptWithMessageKey(message, invalidKey)
	if err == nil {
		t.Error("Expected error for invalid key length, got nil")
	}
	if !strings.Contains(err.Error(), "invalid message key length") {
		t.Errorf("Expected key length error, got: %v", err)
	}
}

func TestDecryptWithMessageKeyInvalidKey(t *testing.T) {
	encrypted := &EncryptedSymmetric{
		Nonce:      "dGVzdA==",
		Ciphertext: "dGVzdA==",
	}

	// Test with invalid key length
	invalidKey := make([]byte, 31)
	_, err := DecryptWithMessageKey(encrypted, invalidKey)
	if err == nil {
		t.Error("Expected error for invalid key length, got nil")
	}
	if !strings.Contains(err.Error(), "invalid message key length") {
		t.Errorf("Expected key length error, got: %v", err)
	}
}

func TestEncryptDecryptMessageKey(t *testing.T) {
	// Generate proper Ed25519 key pair
	ed25519PublicKey, ed25519PrivateKey, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("Failed to generate Ed25519 key pair: %v", err)
	}

	// Generate a message key
	messageKey, err := GenerateMessageKey()
	if err != nil {
		t.Fatalf("Failed to generate message key: %v", err)
	}

	// Encrypt the message key
	encryptedKey, err := EncryptMessageKey(messageKey, ed25519PublicKey)
	if err != nil {
		t.Fatalf("Failed to encrypt message key: %v", err)
	}

	// Verify encrypted key structure
	if encryptedKey.Nonce == "" {
		t.Error("Expected non-empty nonce")
	}
	if encryptedKey.Ciphertext == "" {
		t.Error("Expected non-empty ciphertext")
	}
	if encryptedKey.EphemeralPublicKey == "" {
		t.Error("Expected non-empty ephemeral public key")
	}

	// Decrypt the message key
	decryptedKey, err := DecryptMessageKey(encryptedKey, ed25519PrivateKey)
	if err != nil {
		t.Fatalf("Failed to decrypt message key: %v", err)
	}

	if !bytes.Equal(decryptedKey, messageKey) {
		t.Error("Decrypted message key does not match original")
	}
}

func TestHybridEncryptDecrypt(t *testing.T) {
	// Generate proper Ed25519 key pairs
	ed25519PublicKey1, ed25519PrivateKey1, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("Failed to generate Ed25519 key pair 1: %v", err)
	}
	ed25519PublicKey2, ed25519PrivateKey2, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("Failed to generate Ed25519 key pair 2: %v", err)
	}

	message := "This is a test message for hybrid encryption"
	recipientPublicKeys := []string{
		base64.StdEncoding.EncodeToString(ed25519PublicKey1),
		base64.StdEncoding.EncodeToString(ed25519PublicKey2),
	}

	// Encrypt the message
	hybridMsg, err := HybridEncrypt(message, recipientPublicKeys)
	if err != nil {
		t.Fatalf("Failed to hybrid encrypt: %v", err)
	}

	if hybridMsg.Type != encryptionTypeHybrid {
		t.Errorf("Expected type %q, got %q", encryptionTypeHybrid, hybridMsg.Type)
	}
	if hybridMsg.Payload.Nonce == "" {
		t.Error("Expected non-empty payload nonce")
	}
	if hybridMsg.Payload.Ciphertext == "" {
		t.Error("Expected non-empty payload ciphertext")
	}
	if len(hybridMsg.Keys) != 2 {
		t.Errorf("Expected 2 encrypted keys, got %d", len(hybridMsg.Keys))
	}

	decrypted1, err := decryptHybridMessage(hybridMsg, ed25519PrivateKey1)
	if err != nil {
		t.Fatalf("Failed to decrypt hybrid message with key 1: %v", err)
	}
	if decrypted1 != message {
		t.Errorf("Expected decrypted message %q, got %q", message, decrypted1)
	}

	decrypted2, err := decryptHybridMessage(hybridMsg, ed25519PrivateKey2)
	if err != nil {
		t.Fatalf("Failed to decrypt hybrid message with key 2: %v", err)
	}
	if decrypted2 != message {
		t.Errorf("Expected decrypted message %q, got %q", message, decrypted2)
	}
}

func TestHybridEncryptInvalidPublicKey(t *testing.T) {
	message := testMessage
	recipientPublicKeys := []string{"invalid-base64"}

	_, err := HybridEncrypt(message, recipientPublicKeys)
	if err == nil {
		t.Error("Expected error for invalid public key, got nil")
	}
	if !strings.Contains(err.Error(), "failed to decode public key") {
		t.Errorf("Expected decode error, got: %v", err)
	}
}

func TestDecryptHybridMessageNoValidKeys(t *testing.T) {
	// Generate proper Ed25519 key pairs
	_, ed25519PrivateKey, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("Failed to generate Ed25519 key pair: %v", err)
	}

	otherPublicKey, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("Failed to generate other Ed25519 key pair: %v", err)
	}

	// Create a message encrypted for the other key
	message := testMessage
	recipientPublicKeys := []string{base64.StdEncoding.EncodeToString(otherPublicKey)}
	hybridMsg, err := HybridEncrypt(message, recipientPublicKeys)
	if err != nil {
		t.Fatalf("Failed to create hybrid message: %v", err)
	}

	// Try to decrypt with our private key (should fail)
	_, err = decryptHybridMessage(hybridMsg, ed25519PrivateKey)
	if err == nil {
		t.Error("Expected error when no valid keys available, got nil")
	}
	if !strings.Contains(err.Error(), "failed to decrypt hybrid message with any of the recipient keys") {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func TestParseEncryptedPayload(t *testing.T) {
	// Test parsing hybrid message
	hybridMsg := HybridEncryptedMessage{
		Type: "hybrid",
		Payload: EncryptedSymmetric{
			Nonce:      "test-nonce",
			Ciphertext: "test-ciphertext",
		},
		Keys: map[string]EncryptedMessage{
			"key1": {
				Nonce:              "test-nonce",
				Ciphertext:         "test-ciphertext",
				EphemeralPublicKey: "test-ephemeral-key",
			},
		},
	}

	hybridJSON, err := json.Marshal(hybridMsg)
	if err != nil {
		t.Fatalf("Failed to marshal hybrid message: %v", err)
	}

	parsed, err := parseEncryptedPayload(string(hybridJSON))
	if err != nil {
		t.Fatalf("Failed to parse hybrid payload: %v", err)
	}

	parsedHybrid, ok := parsed.(*HybridEncryptedMessage)
	if !ok {
		t.Error("Expected HybridEncryptedMessage type")
	}
	if parsedHybrid.Type != "hybrid" {
		t.Errorf("Expected type 'hybrid', got %q", parsedHybrid.Type)
	}

	// Test parsing regular encrypted message
	encMsg := EncryptedMessage{
		Nonce:              "test-nonce",
		Ciphertext:         "test-ciphertext",
		EphemeralPublicKey: "test-ephemeral-key",
	}

	encJSON, err := json.Marshal(encMsg)
	if err != nil {
		t.Fatalf("Failed to marshal encrypted message: %v", err)
	}

	parsed, err = parseEncryptedPayload(string(encJSON))
	if err != nil {
		t.Fatalf("Failed to parse encrypted payload: %v", err)
	}

	parsedEnc, ok := parsed.(*EncryptedMessage)
	if !ok {
		t.Error("Expected EncryptedMessage type")
	}
	if parsedEnc.Nonce != "test-nonce" {
		t.Errorf("Expected nonce 'test-nonce', got %q", parsedEnc.Nonce)
	}

	// Test parsing invalid JSON
	_, err = parseEncryptedPayload("invalid-json")
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse encrypted payload") {
		t.Errorf("Expected parse error, got: %v", err)
	}
}

func TestEncryptionRoundTripWithRealKeys(t *testing.T) {
	// Generate proper Ed25519 key pair
	ed25519PublicKey, ed25519PrivateKey, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("Failed to generate Ed25519 key pair: %v", err)
	}

	message := "Real encryption test"

	// Test encryption and decryption
	encrypted, err := EncryptEd25519(message, ed25519PublicKey)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	decrypted, err := DecryptEd25519(encrypted, ed25519PrivateKey)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if decrypted != message {
		t.Errorf("Expected %q, got %q", message, decrypted)
	}
}

func TestSymmetricEncryptionEdgeCases(t *testing.T) {
	// Test with empty message
	messageKey, err := GenerateMessageKey()
	if err != nil {
		t.Fatalf("Failed to generate message key: %v", err)
	}

	encrypted, err := EncryptWithMessageKey("", messageKey)
	if err != nil {
		t.Fatalf("Failed to encrypt empty message: %v", err)
	}

	decrypted, err := DecryptWithMessageKey(encrypted, messageKey)
	if err != nil {
		t.Fatalf("Failed to decrypt empty message: %v", err)
	}

	if decrypted != "" {
		t.Errorf("Expected empty string, got %q", decrypted)
	}

	longMessage := strings.Repeat("A", 10000)
	encrypted, err = EncryptWithMessageKey(longMessage, messageKey)
	if err != nil {
		t.Fatalf("Failed to encrypt long message: %v", err)
	}

	decrypted, err = DecryptWithMessageKey(encrypted, messageKey)
	if err != nil {
		t.Fatalf("Failed to decrypt long message: %v", err)
	}

	if decrypted != longMessage {
		t.Error("Decrypted long message does not match original")
	}
}
