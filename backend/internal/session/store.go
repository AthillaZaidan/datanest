// Package session stores HTTP sessions in the platform database, so they survive a restart.
package session

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

var ErrInvalid = errors.New("invalid session")

const (
	queryTimeout = 5 * time.Second
	// Another request may rotate the same session between our read and our update.
	maxAttempts = 3
)

type Config struct {
	IdleTimeout      time.Duration
	AbsoluteTimeout  time.Duration
	RotationInterval time.Duration
	GracePeriod      time.Duration
	CleanupInterval  time.Duration
}

// Cipher protects the replacement identifier while a rotation is in its grace period.
type Cipher interface {
	Encrypt(plaintext string) ([]byte, error)
	Decrypt(data []byte) (string, error)
}

// Session deliberately stores identity only. Authorization is read from the
// platform database on every request.
type Session struct {
	UserID        uint
	CreatedAt     time.Time
	LastSeen      time.Time
	LastRotatedAt time.Time
	NeedsRotation bool
	CSRFToken     string
}

type Store struct {
	db     *gorm.DB
	cipher Cipher
	cfg    Config
	now    func() time.Time
	// last_seen is written at most this often, not on every request.
	touchEvery time.Duration
	stop       chan struct{}
	once       sync.Once
}

func New(db *gorm.DB, cipher Cipher, cfg Config, clock func() time.Time) *Store {
	if clock == nil {
		clock = time.Now
	}
	s := &Store{
		db: db, cipher: cipher, cfg: cfg, now: clock,
		touchEvery: min(time.Minute, cfg.IdleTimeout/10),
		stop:       make(chan struct{}),
	}
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
	row := models.Session{
		UserID: userID, CurrentHash: key[:], CSRFToken: csrf,
		CreatedAt: now, LastSeen: now, LastRotatedAt: now,
	}
	ctx, cancel := queryContext()
	defer cancel()
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", nil, err
	}
	return raw, toSession(&row), nil
}

// Validate returns a replacement identifier when a cookie must be refreshed.
// The replacement is returned for both a rotation and a valid grace cookie.
func (s *Store) Validate(raw string) (*Session, string, error) {
	sess, next, err := s.validate(raw)
	if err != nil && !errors.Is(err, ErrInvalid) {
		log.Printf("session: validate: %v", err)
	}
	return sess, next, err
}

func (s *Store) validate(raw string) (*Session, string, error) {
	hash := hashRaw(raw)
	ctx, cancel := queryContext()
	defer cancel()

	for range maxAttempts {
		var row models.Session
		err := s.db.WithContext(ctx).Where("current_hash = ? OR previous_hash = ?", hash[:], hash[:]).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrInvalid
		}
		if err != nil {
			return nil, "", err
		}
		now := s.now()
		if !s.live(&row, now) {
			if err := s.db.WithContext(ctx).Delete(&models.Session{}, row.ID).Error; err != nil {
				return nil, "", err
			}
			return nil, "", ErrInvalid
		}
		if !bytes.Equal(row.CurrentHash, hash[:]) {
			return s.viaGrace(ctx, &row, now)
		}

		// A rotation due to a privilege change must not wait, even inside a grace period. The
		// row keeps one predecessor, so an identifier from two rotations ago is dropped.
		due := row.NeedsRotation || now.Sub(row.LastRotatedAt) > s.cfg.RotationInterval
		if !due {
			s.touch(ctx, &row, now)
			return toSession(&row), "", nil
		}

		nextRaw, nextKey, err := newIdentifier()
		if err != nil {
			return nil, "", err
		}
		enc, err := s.cipher.Encrypt(nextRaw)
		if err != nil {
			return nil, "", err
		}
		// Compare-and-set on the identifier we read: exactly one concurrent request wins.
		res := s.db.WithContext(ctx).Model(&models.Session{}).
			Where("id = ? AND current_hash = ?", row.ID, hash[:]).
			Updates(map[string]any{
				"previous_hash":   hash[:],
				"current_hash":    nextKey[:],
				"successor_enc":   enc,
				"grace_until":     now.Add(s.cfg.GracePeriod),
				"last_rotated_at": now,
				"last_seen":       now,
				"needs_rotation":  false,
			})
		if res.Error != nil {
			return nil, "", res.Error
		}
		if res.RowsAffected == 1 {
			row.LastRotatedAt, row.LastSeen, row.NeedsRotation = now, now, false
			return toSession(&row), nextRaw, nil
		}
		// Lost the race: read again. Our identifier is now the previous one, so the next
		// pass takes the grace path and returns the winner's replacement.
	}
	return nil, "", errors.New("session: rotation contention")
}

// viaGrace serves a request that still carries the identifier replaced by a rotation.
func (s *Store) viaGrace(ctx context.Context, row *models.Session, now time.Time) (*Session, string, error) {
	if row.GraceUntil == nil || !now.Before(*row.GraceUntil) || len(row.SuccessorEnc) == 0 {
		return nil, "", ErrInvalid
	}
	next, err := s.cipher.Decrypt(row.SuccessorEnc)
	if err != nil {
		return nil, "", err
	}
	s.touch(ctx, row, now)
	return toSession(row), next, nil
}

func (s *Store) Revoke(raw string) {
	hash := hashRaw(raw)
	ctx, cancel := queryContext()
	defer cancel()
	err := s.db.WithContext(ctx).Where("current_hash = ? OR previous_hash = ?", hash[:], hash[:]).Delete(&models.Session{}).Error
	if err != nil {
		log.Printf("session: revoke: %v", err)
	}
}

func (s *Store) RevokeAllForUser(userID uint) {
	ctx, cancel := queryContext()
	defer cancel()
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.Session{}).Error; err != nil {
		log.Printf("session: revoke all for user %d: %v", userID, err)
	}
}

func (s *Store) MarkNeedsRotation(userID uint) {
	ctx, cancel := queryContext()
	defer cancel()
	err := s.db.WithContext(ctx).Model(&models.Session{}).Where("user_id = ?", userID).Update("needs_rotation", true).Error
	if err != nil {
		log.Printf("session: mark rotation for user %d: %v", userID, err)
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

// Cleanup deletes expired sessions and clears grace data that is no longer usable.
func (s *Store) Cleanup() {
	ctx, cancel := queryContext()
	defer cancel()
	now := s.now()
	err := s.db.WithContext(ctx).
		Where("last_seen < ? OR created_at < ?", now.Add(-s.cfg.IdleTimeout), now.Add(-s.cfg.AbsoluteTimeout)).
		Delete(&models.Session{}).Error
	if err != nil {
		log.Printf("session: cleanup: %v", err)
		return
	}
	err = s.db.WithContext(ctx).Model(&models.Session{}).
		Where("grace_until IS NOT NULL AND grace_until <= ?", now).
		Updates(map[string]any{"previous_hash": nil, "successor_enc": nil, "grace_until": nil}).Error
	if err != nil {
		log.Printf("session: cleanup grace: %v", err)
	}
}

// live uses the stored last_seen, which can lag the real last request by up to touchEvery,
// so a session may expire that much early but never late.
func (s *Store) live(r *models.Session, now time.Time) bool {
	return now.Sub(r.LastSeen) <= s.cfg.IdleTimeout && now.Sub(r.CreatedAt) <= s.cfg.AbsoluteTimeout
}

// touch is best effort: a failed write must not reject a valid request.
func (s *Store) touch(ctx context.Context, row *models.Session, now time.Time) {
	if now.Sub(row.LastSeen) < s.touchEvery {
		return
	}
	err := s.db.WithContext(ctx).Model(&models.Session{}).Where("id = ?", row.ID).Update("last_seen", now).Error
	if err != nil {
		log.Printf("session: touch: %v", err)
		return
	}
	row.LastSeen = now
}

func toSession(r *models.Session) *Session {
	return &Session{
		UserID: r.UserID, CreatedAt: r.CreatedAt, LastSeen: r.LastSeen,
		LastRotatedAt: r.LastRotatedAt, NeedsRotation: r.NeedsRotation, CSRFToken: r.CSRFToken,
	}
}

func queryContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), queryTimeout)
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
