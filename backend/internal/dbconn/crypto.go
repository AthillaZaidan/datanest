package dbconn

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

type Cipher struct{ aead cipher.AEAD }

func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, errors.New("credentials key must be 32 bytes")
	}
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	a, e := cipher.NewGCM(b)
	if e != nil {
		return nil, e
	}
	return &Cipher{a}, nil
}
func (c *Cipher) Encrypt(password string) ([]byte, error) {
	n := make([]byte, c.aead.NonceSize())
	if _, e := rand.Read(n); e != nil {
		return nil, e
	}
	return c.aead.Seal(n, n, []byte(password), nil), nil
}
func (c *Cipher) Decrypt(data []byte) (string, error) {
	n := c.aead.NonceSize()
	if len(data) < n {
		return "", errors.New("invalid encrypted credential")
	}
	p, e := c.aead.Open(nil, data[:n], data[n:], nil)
	return string(p), e
}
