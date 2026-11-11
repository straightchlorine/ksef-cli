package main

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// Ground truth generated with the openssl CLI (what the bash prototype
// used against the real KSeF API):
//
//	printf 'ksef-cli AES parity vector' |
//	  openssl enc -aes-256-cbc -K 000102...1f -iv 000102...0f
func TestAESOpensslParity(t *testing.T) {
	key, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	iv, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f")
	ct, _ := hex.DecodeString("d1df71af199a8f6aa745afc1117d46c6c753e8a04d38dfb9ecf6132d9cbbf5f3")

	pt, err := aesDecrypt(key, iv, ct)
	if err != nil {
		t.Fatal(err)
	}
	if want := "ksef-cli AES parity vector"; string(pt) != want {
		t.Errorf("got %q, want %q", pt, want)
	}
}

func TestAESRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 15, 16, 17, 1000} {
		msg := bytes.Repeat([]byte{0xA5}, n)
		key, iv, ct, err := aesEncrypt(msg)
		if err != nil {
			t.Fatal(err)
		}
		if len(key) != 32 || len(iv) != 16 {
			t.Fatalf("key/iv sizes %d/%d", len(key), len(iv))
		}
		if len(ct) != (n/16+1)*16 {
			t.Errorf("n=%d: ciphertext length %d", n, len(ct))
		}
		pt, err := aesDecrypt(key, iv, ct)
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		if !bytes.Equal(msg, pt) {
			t.Errorf("n=%d: round-trip mismatch", n)
		}
	}
}

func TestPKCS7Unpad(t *testing.T) {
	for _, bad := range [][]byte{
		{},                                      // empty
		bytes.Repeat([]byte{1}, 15),             // not block-aligned
		append(bytes.Repeat([]byte{0}, 15), 0),  // pad byte 0
		append(bytes.Repeat([]byte{0}, 15), 17), // pad byte > block size
		append(bytes.Repeat([]byte{9}, 15), 2),  // inconsistent pad bytes
	} {
		if _, err := pkcs7Unpad(bad, 16); err == nil {
			t.Errorf("pkcs7Unpad(%x) should fail", bad)
		}
	}
}

// printf 'abc' | openssl dgst -sha256 -binary | base64
func TestSHA256B64(t *testing.T) {
	if got, want := sha256B64([]byte("abc")), "ungWv48Bz+pBQUDeXa4iI7ADYaOWF3qctBD/YfIAFa0="; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
