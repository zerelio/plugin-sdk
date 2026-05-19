package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// EncryptionService handles encryption/decryption of sensitive data
type EncryptionService struct {
	masterKey []byte
}

// NewEncryptionService creates a new encryption service
func NewEncryptionService(masterKey []byte) (*EncryptionService, error) {
	if len(masterKey) != 32 {
		return nil, errors.New("master key must be 32 bytes")
	}
	return &EncryptionService{masterKey: masterKey}, nil
}

// NewEncryptionServiceFromString creates service from base64-encoded key
func NewEncryptionServiceFromString(keyBase64 string) (*EncryptionService, error) {
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid key encoding: %w", err)
	}
	return NewEncryptionService(key)
}

// GetMasterKey returns the master key (use with caution, only for key rotation)
func (es *EncryptionService) GetMasterKey() []byte {
	return es.masterKey
}

// Encrypt encrypts plaintext using AES-GCM
func (es *EncryptionService) Encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(es.masterKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext using AES-GCM
func (es *EncryptionService) Decrypt(ciphertextBase64 string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(es.masterKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// EncryptString encrypts a string and returns base64
func (es *EncryptionService) EncryptString(plaintext string) (string, error) {
	return es.Encrypt([]byte(plaintext))
}

// DecryptString decrypts a base64 string
func (es *EncryptionService) DecryptString(ciphertext string) (string, error) {
	plaintext, err := es.Decrypt(ciphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// FieldEncryption handles field-level encryption for database fields
type FieldEncryption struct {
	service *EncryptionService
}

// NewFieldEncryption creates a new field encryption handler
func NewFieldEncryption(service *EncryptionService) *FieldEncryption {
	return &FieldEncryption{service: service}
}

// EncryptField encrypts a sensitive field
func (fe *FieldEncryption) EncryptField(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	return fe.service.EncryptString(value)
}

// DecryptField decrypts a sensitive field
func (fe *FieldEncryption) DecryptField(encryptedValue string) (string, error) {
	if encryptedValue == "" {
		return "", nil
	}
	return fe.service.DecryptString(encryptedValue)
}

// Hash creates a secure hash of data using Argon2
func Hash(data []byte, salt []byte) []byte {
	if salt == nil {
		salt = make([]byte, 16)
		rand.Read(salt)
	}
	// Argon2id parameters: time=3, memory=64MB, threads=4, keyLen=32
	hash := argon2.IDKey(data, salt, 3, 64*1024, 4, 32)
	// Prepend salt to hash for storage
	return append(salt, hash...)
}

// VerifyHash verifies data against a hash
func VerifyHash(data []byte, hash []byte) bool {
	if len(hash) < 16 {
		return false
	}
	salt := hash[:16]
	expectedHash := hash[16:]
	computedHash := argon2.IDKey(data, salt, 3, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(expectedHash, computedHash) == 1
}

// GenerateKey generates a random 32-byte key
func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	return key, err
}

// GenerateKeyString generates a base64-encoded random key
func GenerateKeyString() (string, error) {
	key, err := GenerateKey()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// ChaCha20Poly1305 provides modern authenticated encryption
type ChaCha20Poly1305 struct {
	aead cipher.AEAD
}

// NewChaCha20Poly1305 creates a new ChaCha20-Poly1305 cipher
func NewChaCha20Poly1305(key []byte) (*ChaCha20Poly1305, error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}
	return &ChaCha20Poly1305{aead: aead}, nil
}

// Encrypt encrypts plaintext with associated data
func (c *ChaCha20Poly1305) Encrypt(plaintext, additionalData []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := c.aead.Seal(nonce, nonce, plaintext, additionalData)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext with associated data
func (c *ChaCha20Poly1305) Decrypt(ciphertext, additionalData []byte) ([]byte, error) {
	if len(ciphertext) < c.aead.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce := ciphertext[:c.aead.NonceSize()]
	return c.aead.Open(nil, nonce, ciphertext[c.aead.NonceSize():], additionalData)
}

// SecureString is a string that is automatically encrypted in memory
// and only decrypted when needed
type SecureString struct {
	encrypted string
	es        *EncryptionService
}

// NewSecureString creates a new secure string
func NewSecureString(value string, es *EncryptionService) (*SecureString, error) {
	encrypted, err := es.EncryptString(value)
	if err != nil {
		return nil, err
	}
	return &SecureString{encrypted: encrypted, es: es}, nil
}

// Get decrypts and returns the string value
func (ss *SecureString) Get() (string, error) {
	return ss.es.DecryptString(ss.encrypted)
}

// Clear overwrites the encrypted value (best effort)
func (ss *SecureString) Clear() {
	ss.encrypted = ""
	ss.es = nil
}

// DeriveKeyFromSecret derives a 32-byte encryption key from an arbitrary secret.
// This uses SHA-256 to create a deterministic key from the input secret.
// This is useful for fallback scenarios when a dedicated encryption key is not configured.
func DeriveKeyFromSecret(secret []byte) []byte {
	if len(secret) == 0 {
		// Return a zero key if secret is empty (should not happen in practice)
		return make([]byte, 32)
	}
	h := sha256.Sum256(secret)
	return h[:]
}

// ValidateKey verifies that a key is valid for encryption/decryption operations.
// It does this by performing a test encryption and decryption cycle.
func ValidateKey(key []byte) error {
	if len(key) != 32 {
		return errors.New("key must be 32 bytes")
	}

	// Test encryption and decryption
	es, err := NewEncryptionService(key)
	if err != nil {
		return fmt.Errorf("failed to create encryption service: %w", err)
	}

	testData := []byte("test validation data")
	ciphertext, err := es.Encrypt(testData)
	if err != nil {
		return fmt.Errorf("encryption test failed: %w", err)
	}

	decrypted, err := es.Decrypt(ciphertext)
	if err != nil {
		return fmt.Errorf("decryption test failed: %w", err)
	}

	if string(decrypted) != string(testData) {
		return errors.New("encryption/decryption round-trip failed")
	}

	return nil
}

// RotateKey verifies that both old and new keys are valid and returns information
// about what needs to be re-encrypted. The actual re-encryption must be performed
// at the storage layer where the encrypted data is stored.
//
// This function validates both keys by attempting test operations.
// Returns the number of items that would need re-encryption (0 at this level).
func RotateKey(oldKey, newKey []byte) (int, error) {
	// Validate old key
	if err := ValidateKey(oldKey); err != nil {
		return 0, fmt.Errorf("old key validation failed: %w", err)
	}

	// Validate new key
	if err := ValidateKey(newKey); err != nil {
		return 0, fmt.Errorf("new key validation failed: %w", err)
	}

	// Keys are identical - no rotation needed
	if subtle.ConstantTimeCompare(oldKey, newKey) == 1 {
		return 0, errors.New("new key is identical to old key")
	}

	// At the encryption service level, we don't have access to the stored data.
	// The actual re-encryption happens at the credential store level.
	// This function validates that both keys work correctly.
	return 0, nil
}
