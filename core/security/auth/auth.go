package auth

import (
	"encoding/hex"
	"errors"
	"kyrux/core/security/crypton"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrTokenExpired       = errors.New("token expired")
	ErrTokenRevoked       = errors.New("token revoked")
)

type Claims struct {
	UserID    string
	ExpiresAt time.Time
	JTI       string
}

type Authenticator struct {
	secret  string
	mu      sync.RWMutex
	revoked map[string]time.Time // jti → expiry (para limpeza via gc)
}

func New(secret string) *Authenticator {
	a := &Authenticator{
		secret:  secret,
		revoked: make(map[string]time.Time),
	}
	go a.gc()
	return a
}

// GenerateToken cria um token assinado com userID, expiração e jti único.
// Formato interno: userID|expiresAt|jti
func (a *Authenticator) GenerateToken(userID string, ttl time.Duration) (string, error) {
	jti, err := newJTI()
	if err != nil {
		return "", err
	}
	exp := time.Now().Add(ttl)
	payload := userID + "|" + exp.Format(time.RFC3339) + "|" + jti
	return crypton.Sign(payload, a.secret)
}

// ValidateToken verifica assinatura, expiração e revogação.
func (a *Authenticator) ValidateToken(token string) (*Claims, error) {
	payload, err := crypton.Verify(token, a.secret)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	parts := strings.SplitN(payload, "|", 3)
	if len(parts) != 3 {
		return nil, ErrInvalidCredentials
	}
	userID, expStr, jti := parts[0], parts[1], parts[2]

	exp, err := time.Parse(time.RFC3339, expStr)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if time.Now().After(exp) {
		return nil, ErrTokenExpired
	}

	a.mu.RLock()
	_, isRevoked := a.revoked[jti]
	a.mu.RUnlock()
	if isRevoked {
		return nil, ErrTokenRevoked
	}

	return &Claims{UserID: userID, ExpiresAt: exp, JTI: jti}, nil
}

// RevokeToken invalida o token imediatamente. Chame no logout.
func (a *Authenticator) RevokeToken(claims *Claims) {
	if claims.JTI == "" {
		return
	}
	a.mu.Lock()
	a.revoked[claims.JTI] = claims.ExpiresAt
	a.mu.Unlock()
}

// gc remove entradas expiradas do mapa de revogação periodicamente.
func (a *Authenticator) gc() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		a.mu.Lock()
		now := time.Now()
		for jti, exp := range a.revoked {
			if now.After(exp) {
				delete(a.revoked, jti)
			}
		}
		a.mu.Unlock()
	}
}

func newJTI() (string, error) {
	b, err := crypton.RandomBytes(16)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
