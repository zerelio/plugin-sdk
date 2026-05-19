package crypto

import (
	"encoding/base64"
	"testing"
)

func TestNewEncryptionService_InvalidKeySize(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
	}{
		{"nil key", nil},
		{"empty key", []byte{}},
		{"16 bytes", make([]byte, 16)},
		{"31 bytes", make([]byte, 31)},
		{"33 bytes", make([]byte, 33)},
		{"64 bytes", make([]byte, 64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewEncryptionService(tt.key)
			if err == nil {
				t.Error("expected error for invalid key size")
			}
		})
	}
}

func TestNewEncryptionService_ValidKey(t *testing.T) {
	key := make([]byte, 32)
	svc, err := NewEncryptionService(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestNewEncryptionServiceFromString(t *testing.T) {
	// Valid 32-byte key encoded as base64
	key := make([]byte, 32)
	keyB64 := base64.StdEncoding.EncodeToString(key)

	svc, err := NewEncryptionServiceFromString(keyB64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}

	// Invalid base64
	_, err = NewEncryptionServiceFromString("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}

	// Valid base64 but wrong length (16 bytes)
	shortKey := base64.StdEncoding.EncodeToString(make([]byte, 16))
	_, err = NewEncryptionServiceFromString(shortKey)
	if err == nil {
		t.Error("expected error for wrong key length")
	}
}

func TestEncryptionService_EncryptDecrypt(t *testing.T) {
	svc, err := NewEncryptionService(make([]byte, 32))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{"empty string", ""},
		{"simple text", "hello world"},
		{"json", `{"key": "value", "nested": {"a": 1}}`},
		{"special chars", "!@#$%^&*()_+-=[]{}|;':\",./<>?"},
		{"unicode", "Hello, world! \u4e16\u754c\u306b\u3088\u3046\u3053\u305d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := svc.EncryptString(tt.plaintext)
			if err != nil {
				t.Fatalf("EncryptString error: %v", err)
			}

			// Ciphertext should differ from plaintext
			if tt.plaintext != "" && encrypted == tt.plaintext {
				t.Error("ciphertext should not equal plaintext")
			}

			decrypted, err := svc.DecryptString(encrypted)
			if err != nil {
				t.Fatalf("DecryptString error: %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("DecryptString = %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptionService_EncryptProducesDifferentCiphertexts(t *testing.T) {
	svc, err := NewEncryptionService(make([]byte, 32))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	plaintext := "same input"
	enc1, _ := svc.EncryptString(plaintext)
	enc2, _ := svc.EncryptString(plaintext)

	if enc1 == enc2 {
		t.Error("two encryptions of the same plaintext should produce different ciphertexts (random nonce)")
	}
}

func TestEncryptionService_EncryptDecryptBytes(t *testing.T) {
	svc, err := NewEncryptionService(make([]byte, 32))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	plaintext := []byte("binary data \x00\x01\x02\xff")
	encrypted, err := svc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}

	decrypted, err := svc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypt = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptionService_Decrypt_InvalidCiphertext(t *testing.T) {
	svc, _ := NewEncryptionService(make([]byte, 32))

	tests := []struct {
		name string
		ct   string
	}{
		{"invalid base64", "not-base64!!!"},
		{"too short", base64.StdEncoding.EncodeToString([]byte("short"))},
		{"empty string", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Decrypt(tt.ct)
			if err == nil {
				t.Error("expected error for invalid ciphertext")
			}
		})
	}
}

func TestFieldEncryption(t *testing.T) {
	svc, _ := NewEncryptionService(make([]byte, 32))
	fe := NewFieldEncryption(svc)

	// Empty values pass through
	enc, err := fe.EncryptField("")
	if err != nil {
		t.Fatalf("EncryptField error: %v", err)
	}
	if enc != "" {
		t.Errorf("EncryptField('') = %q, want empty", enc)
	}

	dec, err := fe.DecryptField("")
	if err != nil {
		t.Fatalf("DecryptField error: %v", err)
	}
	if dec != "" {
		t.Errorf("DecryptField('') = %q, want empty", dec)
	}

	// Non-empty values are encrypted
	enc, err = fe.EncryptField("secret-value")
	if err != nil {
		t.Fatalf("EncryptField error: %v", err)
	}
	if enc == "secret-value" {
		t.Error("value should be encrypted")
	}

	dec, err = fe.DecryptField(enc)
	if err != nil {
		t.Fatalf("DecryptField error: %v", err)
	}
	if dec != "secret-value" {
		t.Errorf("DecryptField = %q, want %q", dec, "secret-value")
	}
}

func TestHashAndVerify(t *testing.T) {
	data := []byte("password123")

	// Hash with nil salt (auto-generated)
	hashed := Hash(data, nil)
	if len(hashed) < 16 {
		t.Errorf("hash too short: %d bytes", len(hashed))
	}

	// Verify should succeed
	if !VerifyHash(data, hashed) {
		t.Error("VerifyHash should return true for matching data")
	}

	// Wrong data should fail
	if VerifyHash([]byte("wrong"), hashed) {
		t.Error("VerifyHash should return false for wrong data")
	}

	// Too-short hash should fail
	if VerifyHash(data, []byte("short")) {
		t.Error("VerifyHash should return false for too-short hash")
	}
	if VerifyHash(data, nil) {
		t.Error("VerifyHash should return false for nil hash")
	}
	if VerifyHash(data, []byte{}) {
		t.Error("VerifyHash should return false for empty hash")
	}
}

func TestHash_DeterministicWithSalt(t *testing.T) {
	data := []byte("test-data")
	salt := []byte("fixed-salt-16-by")

	h1 := Hash(data, salt)
	h2 := Hash(data, salt)

	if string(h1) != string(h2) {
		t.Error("Hash should be deterministic with the same salt")
	}
}

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey error: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("GenerateKey returned %d bytes, want 32", len(key))
	}
}

func TestGenerateKeyString(t *testing.T) {
	keyStr, err := GenerateKeyString()
	if err != nil {
		t.Fatalf("GenerateKeyString error: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		t.Fatalf("key is not valid base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("decoded key is %d bytes, want 32", len(decoded))
	}
}

func TestGenerateKey_Uniqueness(t *testing.T) {
	key1, _ := GenerateKey()
	key2, _ := GenerateKey()
	if string(key1) == string(key2) {
		t.Error("two generated keys should be different")
	}
}

func TestDeriveKeyFromSecret(t *testing.T) {
	secret := []byte("my-jwt-secret")

	key1 := DeriveKeyFromSecret(secret)
	key2 := DeriveKeyFromSecret(secret)

	// Same input should produce same key (deterministic)
	if string(key1) != string(key2) {
		t.Error("DeriveKeyFromSecret should be deterministic")
	}

	// Key should be 32 bytes
	if len(key1) != 32 {
		t.Errorf("derived key should be 32 bytes, got %d", len(key1))
	}

	// Different secrets should produce different keys
	secret2 := []byte("different-secret")
	key3 := DeriveKeyFromSecret(secret2)
	if string(key1) == string(key3) {
		t.Error("different secrets should produce different keys")
	}
}

func TestChaCha20Poly1305_EncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	cipher, err := NewChaCha20Poly1305(key)
	if err != nil {
		t.Fatalf("NewChaCha20Poly1305 error: %v", err)
	}

	tests := []struct {
		name    string
		pt      []byte
		ad      []byte
	}{
		{"simple", []byte("hello"), nil},
		{"with aad", []byte("hello"), []byte("additional data")},
		{"empty plaintext", []byte{}, nil},
		{"binary", []byte{0x00, 0x01, 0xff}, []byte("aad")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct, err := cipher.Encrypt(tt.pt, tt.ad)
			if err != nil {
				t.Fatalf("Encrypt error: %v", err)
			}
			pt, err := cipher.Decrypt(ct, tt.ad)
			if err != nil {
				t.Fatalf("Decrypt error: %v", err)
			}
			if string(pt) != string(tt.pt) {
				t.Errorf("Decrypt = %q, want %q", pt, tt.pt)
			}
		})
	}
}

func TestChaCha20Poly1305_WrongAAD(t *testing.T) {
	key := make([]byte, 32)
	cipher, _ := NewChaCha20Poly1305(key)

	ct, _ := cipher.Encrypt([]byte("secret"), []byte("aad1"))
	_, err := cipher.Decrypt(ct, []byte("aad2"))
	if err == nil {
		t.Error("expected error when decrypting with wrong AAD")
	}
}

func TestChaCha20Poly1305_TamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	cipher, _ := NewChaCha20Poly1305(key)

	ct, _ := cipher.Encrypt([]byte("secret"), nil)
	// Tamper with ciphertext
	ct[0] ^= 0xff
	_, err := cipher.Decrypt(ct, nil)
	if err == nil {
		t.Error("expected error when decrypting tampered ciphertext")
	}
}

func TestChaCha20Poly1305_TooShortCiphertext(t *testing.T) {
	key := make([]byte, 32)
	cipher, _ := NewChaCha20Poly1305(key)

	_, err := cipher.Decrypt([]byte("short"), nil)
	if err == nil {
		t.Error("expected error for too-short ciphertext")
	}
}

func TestChaCha20Poly1305_InvalidKey(t *testing.T) {
	_, err := NewChaCha20Poly1305(make([]byte, 16))
	if err == nil {
		t.Error("expected error for invalid key size")
	}
}

func TestSecureString(t *testing.T) {
	svc, _ := NewEncryptionService(make([]byte, 32))

	ss, err := NewSecureString("my-secret", svc)
	if err != nil {
		t.Fatalf("NewSecureString error: %v", err)
	}

	val, err := ss.Get()
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if val != "my-secret" {
		t.Errorf("Get = %q, want %q", val, "my-secret")
	}

	// Clear the secure string
	ss.Clear()

	// After clearing, the encrypted value and service reference are wiped
	// (Get will panic since es is nil, so we verify the clear happened differently)
	if ss.encrypted != "" {
		t.Error("encrypted value should be empty after Clear")
	}
	if ss.es != nil {
		t.Error("encryption service reference should be nil after Clear")
	}
}
