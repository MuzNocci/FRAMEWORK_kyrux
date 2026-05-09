package session

import (
	"encoding/hex"
	"errors"
	"kyrux/core/security/crypton"
	"net/http"
	"sync"
	"time"
)

const (
	DefaultMaxTotal  = 10_000
	DefaultMaxPerKey = 20
)

var (
	ErrStoreFull    = errors.New("session: limite global de sessões atingido")
	ErrKeyLimitFull = errors.New("session: limite de sessões por usuário atingido")
)

type Session struct {
	ID      string
	UserKey string
	Values  map[string]any
	Expires time.Time
}

type Store struct {
	mu        sync.RWMutex
	sessions  map[string]*Session
	byKey     map[string][]string
	ttl       time.Duration
	maxTotal  int
	maxPerKey int
}

func NewStore(ttl time.Duration) *Store {
	s := &Store{
		sessions:  make(map[string]*Session),
		byKey:     make(map[string][]string),
		ttl:       ttl,
		maxTotal:  DefaultMaxTotal,
		maxPerKey: DefaultMaxPerKey,
	}
	go s.gc()
	return s
}

// SetLimits permite ajustar os limites após a criação (ex: via config).
func (s *Store) SetLimits(maxTotal, maxPerKey int) {
	s.mu.Lock()
	s.maxTotal = maxTotal
	s.maxPerKey = maxPerKey
	s.mu.Unlock()
}

// New cria uma sessão sem vínculo a um usuário específico.
func (s *Store) New() (*Session, error) {
	return s.newSession("")
}

// NewForKey cria uma sessão vinculada a uma chave opaca (IP, user ID, etc.)
// e aplica o limite por chave configurado.
func (s *Store) NewForKey(userKey string) (*Session, error) {
	return s.newSession(userKey)
}

func (s *Store) newSession(userKey string) (*Session, error) {
	b, err := crypton.RandomBytes(32)
	if err != nil {
		return nil, err
	}
	id := hex.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.sessions) >= s.maxTotal {
		return nil, ErrStoreFull
	}
	if userKey != "" {
		s.pruneKeyExpired(userKey)
		if len(s.byKey[userKey]) >= s.maxPerKey {
			return nil, ErrKeyLimitFull
		}
	}

	sess := &Session{
		ID:      id,
		UserKey: userKey,
		Values:  make(map[string]any),
		Expires: time.Now().Add(s.ttl),
	}
	s.sessions[id] = sess
	if userKey != "" {
		s.byKey[userKey] = append(s.byKey[userKey], id)
	}
	return sess, nil
}

func (s *Store) Get(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok || time.Now().After(sess.Expires) {
		return nil, false
	}
	return sess, true
}

func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		if sess.UserKey != "" {
			s.removeFromKey(sess.UserKey, id)
		}
		delete(s.sessions, id)
	}
}

// pruneKeyExpired remove IDs expirados do slice de uma chave (deve ser chamado com lock).
func (s *Store) pruneKeyExpired(key string) {
	ids := s.byKey[key]
	alive := ids[:0]
	now := time.Now()
	for _, id := range ids {
		if sess, ok := s.sessions[id]; ok && !now.After(sess.Expires) {
			alive = append(alive, id)
		}
	}
	if len(alive) == 0 {
		delete(s.byKey, key)
	} else {
		s.byKey[key] = alive
	}
}

func (s *Store) removeFromKey(key, id string) {
	ids := s.byKey[key]
	for i, v := range ids {
		if v == id {
			s.byKey[key] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	if len(s.byKey[key]) == 0 {
		delete(s.byKey, key)
	}
}

func (s *Store) gc() {
	ticker := time.NewTicker(s.ttl)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, sess := range s.sessions {
			if now.After(sess.Expires) {
				if sess.UserKey != "" {
					s.removeFromKey(sess.UserKey, id)
				}
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

func CookieName() string { return "kyrux_session" }

// SetCookie define o cookie de sessão com as flags de segurança corretas.
// secure deve ser true em produção (HTTPS). Usar em conjunto com session.New().
func SetCookie(w http.ResponseWriter, sessionID string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName(),
		Value:    sessionID,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})
}

func FromRequest(r *http.Request, store *Store) (*Session, bool) {
	cookie, err := r.Cookie(CookieName())
	if err != nil {
		return nil, false
	}
	return store.Get(cookie.Value)
}
