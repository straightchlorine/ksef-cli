package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"fmt"
	"os"
)

// cmdSelfcheck proves the crypto envelope works on this machine: AES and
// RSA-OAEP round-trips with throwaway keys, then an encrypt against KSeF's
// live published public key. An unreachable KSeF degrades to a warning.
// With a token configured it also logs in, and a failed login is an error.
func cmdSelfcheck() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	msg := []byte("ksef crypto self-check")
	ok := paint(os.Stdout, green, "[  OK  ]")

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
	fmt.Println(ok, "AES-256-CBC round-trip")

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
	fmt.Println(ok, "RSA-OAEP-SHA256 round-trip")

	pub, _, err := publicKeyFor(cfg, "SymmetricKeyEncryption")
	if err != nil {
		fmt.Printf("%s could not fetch KSeF's public key (%s): %v\n",
			paint(os.Stdout, yellow, "[ WARN ]"), envTag(os.Stdout, cfg.Env), err)
		return nil
	}
	if _, err := rsaOAEPEncrypt(pub, msg); err != nil {
		return fmt.Errorf("RSA-OAEP against KSeF's key: %w", err)
	}
	fmt.Printf("%s RSA-OAEP against KSeF's live published public key (%s)\n",
		ok, envTag(os.Stdout, cfg.Env))

	if cfg.Token == "" {
		fmt.Println(paint(os.Stdout, cyan, "[ SKIP ]"), "KSeF login - no token configured")
		hint("Set it with: export KSEF_TOKEN='...'")
		return nil
	}
	if _, err := authenticate(cfg); err != nil {
		return fmt.Errorf("KSeF login: %w", err)
	}
	fmt.Printf("%s KSeF login (%s, nip=%s)\n", ok, envTag(os.Stdout, cfg.Env), cfg.NIP)
	hint("Ready. Next: ksef send FILE.xml, or ksef convert --send INVOICE.json")
	return nil
}
