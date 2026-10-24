// Package session provides the in-memory session store used by the HTTP API.
package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid session")

type Config struct {
	IdleTimeout      time.Duration
	AbsoluteTimeout  time.Duration
	RotationInterval time.Duration
	GracePeriod      time.Duration
	CleanupInterval  time.Duration
}

// Session deliberately stores identity only. Authorization is read from the
// platform database on every request.
type Session struct {
	UserID        uint
	CreatedAt     time.Time
	LastSeen      time.Time
	LastRotatedAt time.Time
	NeedsRotation bool
	Revoked       bool
	CSRFToken     string
	currentKey    [sha256.Size]byte
}

type entry struct {
	session      *Session
	graceUntil   time.Time
	successorRaw string
}

type Store struct {
	mu       sync.Mutex
	cfg      Config
	now      func() time.Time
	entries  map[[sha256.Size]byte]*entry
	userKeys map[uint]map[[sha256.Size]byte]struct{}
	stop     chan struct{}
	once     sync.Once
}

func New(cfg Config, clock func() time.Time) *Store {
	if clock == nil {
		clock = time.Now
	}
	s := &Store{cfg: cfg, now: clock, entries: make(map[[sha256.Size]byte]*entry), userKeys: make(map[uint]map[[sha256.Size]byte]struct{}), stop: make(chan struct{})}
	if cfg.CleanupInterval > 0 {
		go s.cleanupLoop()
	}
	return s
}

func (s *Store) Close() { s.once.Do(func() { close(s.stop) }) }

func (s *Store) Create(userID uint) (string, *Session, error) {
	raw, key, err := newIdentifier()
	if err != nil {
		return "", nil, err
	}
	csrf, _, err := newIdentifier()
	if err != nil {
		return "", nil, err
	}
	now := s.now()
	sess := &Session{UserID: userID, CreatedAt: now, LastSeen: now, LastRotatedAt: now, CSRFToken: csrf, currentKey: key}
	s.mu.Lock()
	s.putLocked(key, &entry{session: sess})
	s.mu.Unlock()
	return raw, sess, nil
}

// Validate returns a replacement identifier when a cookie must be refreshed.
// The replacement is returned for both a rotation and a valid grace cookie.
func (s *Store) Validate(raw string) (*Session, string, error) {
	key := hashRaw(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	e, ok := s.entries[key]
	if !ok || !s.validLocked(e.session, now) {
		if ok {
			s.removeSessionLocked(e.session)
		}
		return nil, "", ErrInvalid
	}
	if !e.graceUntil.IsZero() {
		if !now.Before(e.graceUntil) {
			s.deleteKeyLocked(key)
			return nil, "", ErrInvalid
		}
		e.session.LastSeen = now
		return e.session, e.successorRaw, nil
	}
	if e.session.NeedsRotation || now.Sub(e.session.LastRotatedAt) > s.cfg.RotationInterval {
		nextRaw, nextKey, err := newIdentifier()
		if err != nil {
			return nil, "", err
		}
		e.graceUntil = now.Add(s.cfg.GracePeriod)
		e.successorRaw = nextRaw
		e.session.currentKey = nextKey
		e.session.LastRotatedAt = now
		e.session.NeedsRotation = false
		s.putLocked(nextKey, &entry{session: e.session})
		e.session.LastSeen = now
		return e.session, nextRaw, nil
	}
	e.session.LastSeen = now
	return e.session, "", nil
}

func (s *Store) Revoke(raw string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.entries[hashRaw(raw)]; e != nil {
		e.session.Revoked = true
		s.removeSessionLocked(e.session)
	}
}

func (s *Store) RevokeAllForUser(userID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.userKeys[userID] {
		if e := s.entries[key]; e != nil {
			e.session.Revoked = true
			s.removeSessionLocked(e.session)
		}
	}
}

func (s *Store) MarkNeedsRotation(userID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[*Session]struct{})
	for key := range s.userKeys[userID] {
		if e := s.entries[key]; e != nil {
			if _, ok := seen[e.session]; !ok {
				e.session.NeedsRotation = true
				seen[e.session] = struct{}{}
			}
		}
	}
}

func (s *Store) cleanupLoop() {
	t := time.NewTicker(s.cfg.CleanupInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			s.Cleanup()
		case <-s.stop:
			return
		}
	}
}

func (s *Store) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, e := range s.entries {
		if !s.validLocked(e.session, now) {
			s.removeSessionLocked(e.session)
			continue
		}
		if !e.graceUntil.IsZero() && !now.Before(e.graceUntil) {
			s.deleteKeyLocked(key)
		}
	}
}

func (s *Store) validLocked(sess *Session, now time.Time) bool {
	return !sess.Revoked && now.Sub(sess.LastSeen) <= s.cfg.IdleTimeout && now.Sub(sess.CreatedAt) <= s.cfg.AbsoluteTimeout
}
func (s *Store) putLocked(key [sha256.Size]byte, e *entry) {
	s.entries[key] = e
	if s.userKeys[e.session.UserID] == nil {
		s.userKeys[e.session.UserID] = make(map[[sha256.Size]byte]struct{})
	}
	s.userKeys[e.session.UserID][key] = struct{}{}
}
func (s *Store) deleteKeyLocked(key [sha256.Size]byte) {
	if e := s.entries[key]; e != nil {
		delete(s.entries, key)
		delete(s.userKeys[e.session.UserID], key)
		if len(s.userKeys[e.session.UserID]) == 0 {
			delete(s.userKeys, e.session.UserID)
		}
	}
}
func (s *Store) removeSessionLocked(sess *Session) {
	for key, e := range s.entries {
		if e.session == sess {
			s.deleteKeyLocked(key)
		}
	}
}
func newIdentifier() (string, [sha256.Size]byte, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", [sha256.Size]byte{}, err
	}
	raw := base64.RawURLEncoding.EncodeToString(b[:])
	return raw, hashRaw(raw), nil
}
func hashRaw(raw string) [sha256.Size]byte { return sha256.Sum256([]byte(raw)) }
