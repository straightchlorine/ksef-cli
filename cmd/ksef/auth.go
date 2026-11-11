package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// statusInfo is the status object KSeF returns for async.
// 100 means in progress, 200 success, anything else failure
type statusInfo struct {
	Code        int      `json:"code"`
	Description string   `json:"description"`
	Details     []string `json:"details"`
	// set on some failures, e.g. originalKsefNumber for a duplicate invoice
	Extensions map[string]string `json:"extensions"`
}

var (
	pollInterval = time.Second
	pollAttempts = 60
)

// authenticate exchanges the KSeF token for a short-lived access token:
// challenge, encrypted token submit, status poll, redeem.
//
// TODO: consider cachin in cfg.CacheDir if it's too slow
func authenticate(cfg *Config) (string, error) {
	if cfg.NIP == "" || cfg.Token == "" {
		return "", errors.New("nip and ksef_token (or $KSEF_TOKEN) must be set")
	}
	base := baseURLs[cfg.Env]

	var challenge struct {
		Challenge   string `json:"challenge"`
		TimestampMs int64  `json:"timestampMs"`
	}
	if err := apiJSON("POST", base+"/auth/challenge", "", nil, &challenge); err != nil {
		return "", err
	}

	pub, keyID, err := publicKeyFor(cfg, "KsefTokenEncryption")
	if err != nil {
		return "", err
	}
	plain := cfg.Token + "|" + strconv.FormatInt(challenge.TimestampMs, 10)
	encrypted, err := rsaOAEPEncrypt(pub, []byte(plain))
	if err != nil {
		return "", err
	}

	var started struct {
		ReferenceNumber     string `json:"referenceNumber"`
		AuthenticationToken struct {
			Token string `json:"token"`
		} `json:"authenticationToken"`
	}
	err = apiJSON("POST", base+"/auth/ksef-token", "", map[string]any{
		"challenge":         challenge.Challenge,
		"contextIdentifier": map[string]string{"type": "Nip", "value": cfg.NIP},
		"encryptedToken":    base64.StdEncoding.EncodeToString(encrypted),
		"publicKeyId":       keyID,
	}, &started)
	if err != nil {
		return "", err
	}
	authToken := started.AuthenticationToken.Token

	var op struct {
		Status statusInfo `json:"status"`
	}
	statusURL := base + "/auth/" + started.ReferenceNumber
	for i := 0; ; i++ {
		if err := apiJSON("GET", statusURL, authToken, nil, &op); err != nil {
			return "", err
		}
		if op.Status.Code != 100 {
			break
		}
		if i == pollAttempts {
			return "", errors.New("login still in progress, giving up - try again in a moment")
		}
		time.Sleep(pollInterval)
	}
	if op.Status.Code != 200 {
		return "", fmt.Errorf(
			"login rejected: %d %s %v", op.Status.Code, op.Status.Description, op.Status.Details,
		)
	}

	var tokens struct {
		AccessToken struct {
			Token string `json:"token"`
		} `json:"accessToken"`
	}
	if err := apiJSON("POST", base+"/auth/token/redeem", authToken, nil, &tokens); err != nil {
		return "", err
	}
	if tokens.AccessToken.Token == "" {
		return "", errors.New("login succeeded but KSeF returned no access token")
	}
	return tokens.AccessToken.Token, nil
}
