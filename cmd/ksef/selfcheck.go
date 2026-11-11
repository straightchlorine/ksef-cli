package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"fmt"
)

// cmdSelfcheck proves the crypto envelope works on this machine: AES and
// RSA-OAEP round-trips with throwaway keys, then an encrypt against KSeF's
// live published public key. The network step degrades to a warning - the
// local crypto verdict is the point.
func cmdSelfcheck() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	msg := []byte("ksef crypto self-check")

	key, iv, ct, err := aesEncrypt(msg)
	if err != nil {
		return fmt.Errorf("AES encrypt: %w", err)
	}
	pt, err := aesDecrypt(key, iv, ct)
	if err != nil {
		return fmt.Errorf("AES decrypt: %w", err)
	}
	if !bytes.Equal(msg, pt) {
		return fmt.Errorf("AES round-trip mismatch")
	}
	fmt.Println("[  OK  ] AES-256-CBC round-trip")

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("RSA keygen: %w", err)
	}
	enc, err := rsaOAEPEncrypt(&rsaKey.PublicKey, msg)
	if err != nil {
		return fmt.Errorf("RSA-OAEP encrypt: %w", err)
	}
	dec, err := rsa.DecryptOAEP(sha256.New(), nil, rsaKey, enc, nil)
	if err != nil {
		return fmt.Errorf("RSA-OAEP decrypt: %w", err)
	}
	if !bytes.Equal(msg, dec) {
		return fmt.Errorf("RSA-OAEP round-trip mismatch")
	}
	fmt.Println("[  OK  ] RSA-OAEP-SHA256 round-trip")

	pub, err := publicKeyFor(cfg, "SymmetricKeyEncryption")
	if err != nil {
		fmt.Printf("[ WARN ] could not fetch KSeF's public key (env=%s): %v\n", cfg.Env, err)
		return nil
	}
	if _, err := rsaOAEPEncrypt(pub, msg); err != nil {
		return fmt.Errorf("RSA-OAEP against KSeF's key: %w", err)
	}
	fmt.Printf("[  OK  ] RSA-OAEP against KSeF's live published public key (env=%s)\n", cfg.Env)
	return nil
}
