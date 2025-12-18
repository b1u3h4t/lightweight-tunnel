package crypto

import (
	"bytes"
	"testing"
)

func TestNewCipher(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid key", "my-secret-key", false},
		{"empty key", "", true},
		{"short key", "a", false},
		{"long key", "this-is-a-very-long-key-that-exceeds-32-characters", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cipher, err := NewCipher(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewCipher() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("NewCipher() unexpected error: %v", err)
				return
			}
			if cipher == nil {
				t.Errorf("NewCipher() returned nil cipher")
			}
		})
	}
}

func TestEncryptDecrypt(t *testing.T) {
	cipher, err := NewCipher("test-key-123")
	if err != nil {
		t.Fatalf("Failed to create cipher: %v", err)
	}

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"short message", []byte("hello")},
		{"empty message", []byte{}},
		{"long message", bytes.Repeat([]byte("A"), 1000)},
		{"binary data", []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt
			ciphertext, err := cipher.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error: %v", err)
			}

			// Verify ciphertext is different from plaintext (except for empty)
			if len(tt.plaintext) > 0 && bytes.Equal(ciphertext, tt.plaintext) {
				t.Errorf("Encrypt() ciphertext equals plaintext")
			}

			// Decrypt
			decrypted, err := cipher.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt() error: %v", err)
			}

			// Verify decrypted matches original
			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Errorf("Decrypt() = %v, want %v", decrypted, tt.plaintext)
			}
		})
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	cipher1, _ := NewCipher("key-1")
	cipher2, _ := NewCipher("key-2")

	plaintext := []byte("secret message")

	// Encrypt with cipher1
	ciphertext, err := cipher1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	// Try to decrypt with cipher2
	_, err = cipher2.Decrypt(ciphertext)
	if err == nil {
		t.Errorf("Decrypt() with wrong key should fail")
	}
}

func TestDecryptInvalidData(t *testing.T) {
	cipher, _ := NewCipher("test-key")

	tests := []struct {
		name       string
		ciphertext []byte
	}{
		{"too short", []byte{0x01, 0x02}},
		{"random data", []byte("this is not encrypted data at all!!")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cipher.Decrypt(tt.ciphertext)
			if err == nil {
				t.Errorf("Decrypt() should fail for invalid data")
			}
		})
	}
}

func TestOverhead(t *testing.T) {
	cipher, _ := NewCipher("test-key")
	
	overhead := cipher.Overhead()
	
	// AES-GCM: 12-byte nonce + 16-byte tag = 28 bytes
	expectedOverhead := 28
	if overhead != expectedOverhead {
		t.Errorf("Overhead() = %d, want %d", overhead, expectedOverhead)
	}
}

func TestDifferentCiphertextsSamePlaintext(t *testing.T) {
	cipher, _ := NewCipher("test-key")
	plaintext := []byte("same message")

	// Encrypt twice - should produce different ciphertexts due to random nonce
	ciphertext1, _ := cipher.Encrypt(plaintext)
	ciphertext2, _ := cipher.Encrypt(plaintext)

	if bytes.Equal(ciphertext1, ciphertext2) {
		t.Errorf("Same plaintext should produce different ciphertexts (random nonce)")
	}

	// Both should decrypt correctly
	decrypted1, _ := cipher.Decrypt(ciphertext1)
	decrypted2, _ := cipher.Decrypt(ciphertext2)

	if !bytes.Equal(decrypted1, plaintext) || !bytes.Equal(decrypted2, plaintext) {
		t.Errorf("Both ciphertexts should decrypt to original plaintext")
	}
}
