package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
)

// NewAEAD 从任意长度的密钥派生 32 字节 AES-256-GCM 密钥。
func NewAEAD(key []byte) (cipher.AEAD, error) {
	hash := sha256.Sum256(key)
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Encrypt 加密明文，返回 hex 编码的密文（含 nonce）。
func Encrypt(aead cipher.AEAD, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// Decrypt 解密由 Encrypt 产生的密文；如果密文字符串为空则返回空。
func Decrypt(aead cipher.AEAD, encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	ciphertext, err := hex.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	nonceSize := aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
