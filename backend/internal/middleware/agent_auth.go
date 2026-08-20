package middleware

// agent_auth.go — per-agent cryptographic identity for NeuroOps agents.
//
// Bootstrap (registration):  agent presents a single-use enrollment token.
// Steady-state (ingest):      every request is HMAC-SHA256 signed with the agent's
//                             unique secret; a timestamp + nonce prevent replay.
//
// Signature canonical form:
//   METHOD\nagent_id\ntimestamp_unix\nnonce\nhex(sha256(body))
// Signed with: HMAC-SHA256(raw_agent_secret, canonical_string)
// Header set:  X-Agent-ID, X-Timestamp, X-Nonce, X-Signature

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// ─── Context helpers ────────────────────────────────────────────────────────

type agentContextKey struct{}

// VerifiedAgent is injected into the request context by AgentAuthenticator.Middleware
// after successful HMAC verification.
type VerifiedAgent struct {
	AgentID  string
	TenantID string
}

// AgentFromContext retrieves the verified agent stamped by the HMAC middleware.
func AgentFromContext(r *http.Request) (*VerifiedAgent, bool) {
	v, ok := r.Context().Value(agentContextKey{}).(*VerifiedAgent)
	return v, ok && v != nil
}

// ─── Secret key derivation ──────────────────────────────────────────────────

// DeriveAgentSecretKey returns a 32-byte AES-256 key.
// If agentSecretKey is a non-empty 32+ char string it is SHA-256 hashed to produce
// a stable 32-byte key; otherwise the key is derived from the JWT secret with a
// domain separator so the two keys are always independent.
func DeriveAgentSecretKey(jwtSecret, agentSecretKey string) []byte {
	h := sha256.New()
	if len(agentSecretKey) >= 32 {
		h.Write([]byte("aiops:agent-enc:v1:"))
		h.Write([]byte(agentSecretKey))
	} else {
		// Fall back: derive from JWT secret (still independent via domain separator).
		h.Write([]byte("aiops:agent-enc:v1:jwt-derived:"))
		h.Write([]byte(jwtSecret))
	}
	return h.Sum(nil)
}

// ─── AES-256-GCM helpers ────────────────────────────────────────────────────

// EncryptAgentSecret encrypts a raw agent secret using AES-256-GCM.
// Returns base64(nonce || ciphertext).
func EncryptAgentSecret(key []byte, rawSecret string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm init: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("nonce gen: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(rawSecret), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptAgentSecret decrypts an AES-256-GCM encrypted agent secret.
func DecryptAgentSecret(key []byte, enc string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm init: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}

// GenerateAgentSecret returns a cryptographically random agent secret in the
// form "agt_<64 hex chars>" (256 bits of entropy).
func GenerateAgentSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "agt_" + hex.EncodeToString(b), nil
}

// ─── Nonce cache (in-process, replay protection) ────────────────────────────

// nonceCache tracks used nonces for a 10-minute window.
// For multi-instance deployments replace this with a Redis SET + TTL.
type nonceCache struct {
	mu      sync.Mutex
	entries map[string]time.Time // "agentID:nonce" → expiry
}

func newNonceCache() *nonceCache {
	return &nonceCache{entries: make(map[string]time.Time)}
}

// use atomically checks and records a nonce. Returns true on first use, false on replay.
func (c *nonceCache) use(agentID, nonce string) bool {
	key := agentID + ":" + nonce
	expiry := time.Now().Add(10 * time.Minute)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; exists {
		return false
	}
	c.entries[key] = expiry
	return true
}

func (c *nonceCache) cleanup() {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, exp := range c.entries {
		if now.After(exp) {
			delete(c.entries, k)
		}
	}
}

// ─── Agent secret store interface ───────────────────────────────────────────

// agentSecretReader is the subset of store.AgentStore needed by the authenticator.
// Using an interface keeps the middleware package free of a concrete store import.
type agentSecretReader interface {
	GetAgentByID(id string) (*models.Agent, error)
	GetSecretEnc(agentID string) (string, error)
}

// ─── Authenticator ──────────────────────────────────────────────────────────

// AgentAuthenticator holds shared state (nonce cache, secret key) for the
// HMAC middleware. Construct once at startup via NewAgentAuthenticator.
type AgentAuthenticator struct {
	store      agentSecretReader
	secretKey  []byte // 32-byte AES-256 key
	devToken   string // legacy AGENT_INGEST_TOKEN; non-empty enables old-style fallback
	isProd     bool
	nonceCache *nonceCache
}

// NewAgentAuthenticator creates the authenticator and starts the nonce cleanup goroutine.
func NewAgentAuthenticator(store agentSecretReader, secretKey []byte, devToken string, isProd bool) *AgentAuthenticator {
	a := &AgentAuthenticator{
		store:      store,
		secretKey:  secretKey,
		devToken:   devToken,
		isProd:     isProd,
		nonceCache: newNonceCache(),
	}
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			a.nonceCache.cleanup()
		}
	}()
	return a
}

// Middleware wraps an ingest handler with per-agent HMAC authentication.
//
// Auth precedence (evaluated in order):
//  1. Open dev mode: isProd=false AND devToken="" → pass-through (no auth)
//  2. Legacy dev mode: X-Agent-Token matches devToken → pass-through (backward compat)
//  3. HMAC mode: validate X-Agent-ID + X-Timestamp + X-Nonce + X-Signature
func (a *AgentAuthenticator) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Open dev mode: completely unconfigured, non-production environment.
		if !a.isProd && a.devToken == "" {
			next(w, r)
			return
		}

		// Legacy dev mode: old-style shared token still accepted for local dev agents.
		if a.devToken != "" && r.Header.Get("X-Agent-Token") == a.devToken {
			next(w, r)
			return
		}

		// HMAC mode — required for all other cases.
		agentID := r.Header.Get("X-Agent-ID")
		tsStr := r.Header.Get("X-Timestamp")
		nonce := r.Header.Get("X-Nonce")
		sig := r.Header.Get("X-Signature")

		if agentID == "" || tsStr == "" || nonce == "" || sig == "" {
			writeAuthError(w, "missing required headers: X-Agent-ID, X-Timestamp, X-Nonce, X-Signature")
			return
		}

		// Validate timestamp window (±5 minutes).
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil || absInt(time.Now().Unix()-ts) > 300 {
			writeAuthError(w, "X-Timestamp out of acceptable window (±5 min)")
			return
		}

		// Nonce must be at least 32 hex chars (128 bits).
		if len(nonce) < 32 {
			writeAuthError(w, "X-Nonce too short (minimum 32 hex characters)")
			return
		}

		// Buffer body so we can hash it and still pass it downstream.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeAuthError(w, "failed to read request body")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		// Look up agent — reject unknown agents immediately.
		agent, err := a.store.GetAgentByID(agentID)
		if err != nil || agent == nil {
			writeAuthError(w, "unknown agent")
			return
		}

		// Retrieve and decrypt the stored per-agent secret.
		secretEnc, err := a.store.GetSecretEnc(agentID)
		if err != nil || secretEnc == "" {
			writeAuthError(w, "agent not cryptographically enrolled; re-register to obtain a signing secret")
			return
		}
		rawSecret, err := DecryptAgentSecret(a.secretKey, secretEnc)
		if err != nil {
			writeAuthError(w, "internal error: secret decryption failed")
			return
		}

		// Compute body SHA-256 and canonical message.
		bodySum := sha256.Sum256(body)
		canonical := r.Method + "\n" + agentID + "\n" + tsStr + "\n" + nonce + "\n" + hex.EncodeToString(bodySum[:])

		// Compute expected HMAC and compare in constant time.
		mac := hmac.New(sha256.New, []byte(rawSecret))
		mac.Write([]byte(canonical))
		expected := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(expected), []byte(sig)) {
			writeAuthError(w, "invalid signature")
			return
		}

		// Atomically record the nonce — prevents replay even under concurrent requests.
		if !a.nonceCache.use(agentID, nonce) {
			writeAuthError(w, "nonce already used (replay detected)")
			return
		}

		// Inject verified identity into context so handlers don't need another DB round-trip.
		ctx := context.WithValue(r.Context(), agentContextKey{}, &VerifiedAgent{
			AgentID:  agentID,
			TenantID: agent.TenantID,
		})
		next(w, r.WithContext(ctx))
	}
}

// ─── Internal helpers ────────────────────────────────────────────────────────

func writeAuthError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}

func absInt(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
