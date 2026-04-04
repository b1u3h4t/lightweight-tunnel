package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"sync/atomic"
)

// Cipher provides encryption and decryption using AES-GCM
type Cipher struct {
	aead    cipher.AEAD
	prefix  [4]byte // random prefix set at init time, unique per Cipher instance
	counter uint64  // atomic counter for nonce generation; combined with prefix to form unique nonce
}

// NewCipher creates a new cipher from a key string
// The key is hashed with SHA-256 to produce a 256-bit AES key
func NewCipher(key string) (*Cipher, error) {
	if key == "" {
		return nil, errors.New("key cannot be empty")
	}

	// Hash the key to get a 256-bit key
	hash := sha256.Sum256([]byte(key))

	// Create AES cipher
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return nil, err
	}

	// Create GCM mode
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	c := &Cipher{aead: aead}

	// Generate a random 4-byte prefix for nonce uniqueness across restarts
	if _, err := io.ReadFull(rand.Reader, c.prefix[:]); err != nil {
		return nil, err
	}

	return c, nil
}

// Encrypt encrypts plaintext and returns ciphertext with nonce prepended
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	// Build nonce from 4-byte random prefix + 8-byte atomic counter
	// This avoids per-packet syscall to rand.Reader while guaranteeing uniqueness
	nonce := make([]byte, c.aead.NonceSize()) // 12 bytes for AES-GCM
	copy(nonce[:4], c.prefix[:])
	binary.LittleEndian.PutUint64(nonce[4:], atomic.AddUint64(&c.counter, 1))

	// Encrypt and prepend nonce
	ciphertext := c.aead.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext (with prepended nonce) and returns plaintext
func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < c.aead.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}

	// Extract nonce
	nonce := ciphertext[:c.aead.NonceSize()]
	ciphertext = ciphertext[c.aead.NonceSize():]

	// Decrypt
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// Overhead returns the total overhead added by encryption (nonce + tag)
func (c *Cipher) Overhead() int {
	return c.aead.NonceSize() + c.aead.Overhead()
}
