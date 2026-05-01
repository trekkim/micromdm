package server

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/micromdm/micromdm/dep"
	"github.com/micromdm/micromdm/platform/config"
)

// LocalGetTokenSigner implements mdm.GetToken by signing the MAID JWT
// directly using the DEP private key already stored in MicroMDM's BoltDB.
// No external service is required.
type LocalGetTokenSigner struct {
	store  config.Store
	dep    *dep.Client

	mu         sync.Mutex
	serverUUID string // cached after first successful lookup
}

// NewLocalGetTokenSigner creates a signer that reads the DEP private key
// from the MicroMDM config store and looks up the server UUID via the DEP
// AccountDetail API (result is cached for the lifetime of the process).
func NewLocalGetTokenSigner(store config.Store, depClient *dep.Client) *LocalGetTokenSigner {
	return &LocalGetTokenSigner{store: store, dep: depClient}
}

// GetToken generates and returns a signed MAID JWT as raw bytes.
// The returned bytes are placed directly into the TokenData plist key.
func (s *LocalGetTokenSigner) GetToken(_ context.Context, _, _ string) ([]byte, error) {
	key, _, err := s.store.DEPKeypair()
	if err != nil {
		return nil, fmt.Errorf("load DEP private key: %w", err)
	}

	serverUUID, err := s.cachedServerUUID()
	if err != nil {
		return nil, fmt.Errorf("get server UUID: %w", err)
	}

	token, err := signMAIDJWT(key, serverUUID)
	if err != nil {
		return nil, fmt.Errorf("sign MAID JWT: %w", err)
	}

	return []byte(token), nil
}

// cachedServerUUID returns the ABM server UUID, fetching it once via the
// DEP AccountDetail API and caching the result.
func (s *LocalGetTokenSigner) cachedServerUUID() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.serverUUID != "" {
		return s.serverUUID, nil
	}
	account, err := s.dep.Account()
	if err != nil {
		return "", fmt.Errorf("DEP account detail: %w", err)
	}
	if account.ServerUUID == "" {
		return "", fmt.Errorf("DEP account returned empty server_uuid")
	}
	s.serverUUID = account.ServerUUID
	return s.serverUUID, nil
}

// signMAIDJWT builds and signs a compact RS256 JWT for com.apple.maid.
// Implements the JWT spec manually to avoid adding a new dependency.
func signMAIDJWT(key *rsa.PrivateKey, serverUUID string) (string, error) {
	// Header
	headerJSON, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", err
	}

	// Claims
	claimsJSON, err := json.Marshal(map[string]interface{}{
		"iss":          serverUUID,
		"iat":          time.Now().Unix(),
		"jti":          uuid.New().String(),
		"service_type": "com.apple.maid",
	})
	if err != nil {
		return "", err
	}

	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	claims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := header + "." + claims

	h := sha256.New()
	h.Write([]byte(signingInput))
	digest := h.Sum(nil)

	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest)
	if err != nil {
		return "", fmt.Errorf("RSA sign: %w", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
