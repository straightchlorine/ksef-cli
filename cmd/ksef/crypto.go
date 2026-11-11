// KSeF's client-side crypto envelope: invoices are AES-256-CBC encrypted
// with a fresh random key+IV per invoice, and the AES key travels to KSeF
// wrapped with RSA-OAEP (SHA-256, MGF1-SHA256) under a server-published
// public key.
// Hashes go into request bodies as base64 SHA-256.
package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

// aesEncrypt encrypts plaintext with a fresh random 256-bit key and 128-bit IV
// (AES-256-CBC, PKCS7) and returns key, iv, ciphertext.
func aesEncrypt(plaintext []byte) (key, iv, ciphertext []byte, err error) {
	key = make([]byte, 32)
	iv = make([]byte, 16)
	if _, err = rand.Read(key); err != nil {
		return nil, nil, nil, err
	}
	if _, err = rand.Read(iv); err != nil {
		return nil, nil, nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, nil, err
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext = make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)
	return key, iv, ciphertext, nil
}

func aesDecrypt(key, iv, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf(
			"ciphertext length %d is not a multiple of the AES block size",
			len(ciphertext),
		)
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)
	return pkcs7Unpad(plaintext, aes.BlockSize)
}

func pkcs7Pad(b []byte, blockSize int) []byte {
	n := blockSize - len(b)%blockSize
	return append(append([]byte{}, b...), bytes.Repeat([]byte{byte(n)}, n)...)
}

func pkcs7Unpad(b []byte, blockSize int) ([]byte, error) {
	if len(b) == 0 || len(b)%blockSize != 0 {
		return nil, errors.New("invalid PKCS7 padding: bad length")
	}
	n := int(b[len(b)-1])
	if n == 0 || n > blockSize {
		return nil, errors.New("invalid PKCS7 padding: bad pad byte")
	}
	for _, c := range b[len(b)-n:] {
		if int(c) != n {
			return nil, errors.New("invalid PKCS7 padding: inconsistent pad bytes")
		}
	}
	return b[:len(b)-n], nil
}

// rsaOAEPEncrypt is RSA-OAEP with SHA-256; crypto/rsa uses the same hash
// for MGF1 by default, which is exactly the padding KSeF's docs specify.
func rsaOAEPEncrypt(pub *rsa.PublicKey, plaintext []byte) ([]byte, error) {
	return rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, plaintext, nil)
}

// sha256B64 is the base64 SHA-256 digest KSeF request bodies carry
// alongside file sizes, for both plaintext and ciphertext.
func sha256B64(b []byte) string {
	sum := sha256.Sum256(b)
	return base64.StdEncoding.EncodeToString(sum[:])
}
