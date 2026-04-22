package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// Crypter encrypts and decrypts short strings (e.g. stored DB passwords) with
// AES-256-GCM, prefixing the nonce. Keys must be exactly 32 bytes.
type Crypter struct {
	gcm cipher.AEAD
}

func NewCrypter(key []byte) (*Crypter, error) {
	if len(key) != 32 {
		return nil, errors.New("app_key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Crypter{gcm: gcm}, nil
}

func (c *Crypter) Encrypt(plain string) (string, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := c.gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

func (c *Crypter) Decrypt(token string) (string, error) {
	if token == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", err
	}
	ns := c.gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	out, err := c.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
