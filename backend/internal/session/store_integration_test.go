//go:build integration

package session

import (
	"bytes"
	"crypto/sha256"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

var testConfig = Config{IdleTimeout: 30 * time.Minute, AbsoluteTimeout: 8 * time.Hour, RotationInterval: 15 * time.Minute, GracePeriod: 30 * time.Second}

func startTime() time.Time { return time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC) }

func newStore(t *testing.T, db *gorm.DB, now *time.Time) *Store {
	t.Helper()
	cipher, err := dbconn.NewCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, cipher, testConfig, func() time.Time { return *now })
	t.Cleanup(s.Close)
	return s
}

// setup returns a store on an empty database with one user.
func setup(t *testing.T, now *time.Time) (*Store, *gorm.DB, models.User) {
	t.Helper()
	db := testutil.PlatformDB(t)
	return newStore(t, db, now), db, testutil.CreateUser(t, db, "ayu")
}

func loadRow(t *testing.T, db *gorm.DB, userID uint) models.Session {
	t.Helper()
	var row models.Session
	if err := db.Where("user_id = ?", userID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func count(t *testing.T, db *gorm.DB, userID uint) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&models.Session{}).Where("user_id = ?", userID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateAndValidate(t *testing.T) {
	now := startTime()
	s, _, u := setup(t, &now)
	raw, created, err := s.Create(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, next, err := s.Validate(raw)
	if err != nil || got.UserID != u.ID || next != "" || got.CSRFToken == "" || got.CSRFToken != created.CSRFToken {
		t.Fatalf("Validate = %#v, %q, %v", got, next, err)
	}
	if _, _, err := s.Validate("not-a-session"); err != ErrInvalid {
		t.Fatalf("unknown identifier = %v", err)
	}
}

func TestOnlyHashesAreStored(t *testing.T) {
	now := startTime()
	s, db, u := setup(t, &now)
	raw, _, _ := s.Create(u.ID)
	row := loadRow(t, db, u.ID)
	want := sha256.Sum256([]byte(raw))
	if !bytes.Equal(row.CurrentHash, want[:]) || bytes.Contains(row.CurrentHash, []byte(raw)) {
		t.Fatal("current identifier is not stored as its SHA-256 hash")
	}

	now = now.Add(16 * time.Minute)
	_, next, err := s.Validate(raw)
	if err != nil || next == "" {
		t.Fatalf("rotation = %q, %v", next, err)
	}
	row = loadRow(t, db, u.ID)
	wantNext := sha256.Sum256([]byte(next))
	if !bytes.Equal(row.CurrentHash, wantNext[:]) || !bytes.Equal(row.PreviousHash, want[:]) {
		t.Fatal("rotation did not move the hashes")
	}
	if len(row.SuccessorEnc) == 0 || bytes.Contains(row.SuccessorEnc, []byte(next)) {
		t.Fatal("replacement identifier must be stored encrypted, not in plaintext")
	}
}

func TestIdleExpiry(t *testing.T) {
	now := startTime()
	s, _, u := setup(t, &now)
	raw, _, _ := s.Create(u.ID)
	now = now.Add(31 * time.Minute)
	if _, _, err := s.Validate(raw); err != ErrInvalid {
		t.Fatalf("idle error = %v", err)
	}
}

// The absolute timeout counts from creation: activity and rotation must not extend it.
func TestAbsoluteExpiryDespiteActivity(t *testing.T) {
	now := startTime()
	s, _, u := setup(t, &now)
	raw, _, _ := s.Create(u.ID)
	for range 24 { // 8h in 20-minute steps, active the whole time
		now = now.Add(20 * time.Minute)
		_, next, err := s.Validate(raw)
		if err != nil {
			t.Fatalf("active session rejected at %v: %v", now.Sub(startTime()), err)
		}
		if next != "" {
			raw = next
		}
	}
	now = now.Add(time.Minute)
	if _, _, err := s.Validate(raw); err != ErrInvalid {
		t.Fatalf("absolute error = %v", err)
	}
}

func TestRotationAndGrace(t *testing.T) {
	now := startTime()
	s, _, u := setup(t, &now)
	raw, sess, _ := s.Create(u.ID)
	now = now.Add(16 * time.Minute)
	got, next, err := s.Validate(raw)
	if err != nil || next == "" {
		t.Fatalf("rotation = %q, %v", next, err)
	}
	if !got.CreatedAt.Equal(sess.CreatedAt) {
		t.Fatal("rotation extended absolute lifetime")
	}
	_, replay, err := s.Validate(raw)
	if err != nil || replay != next {
		t.Fatalf("grace = %q, %v", replay, err)
	}
	if _, again, err := s.Validate(next); err != nil || again != "" {
		t.Fatalf("successor = %q, %v", again, err)
	}
	now = now.Add(31 * time.Second)
	if _, _, err := s.Validate(raw); err != ErrInvalid {
		t.Fatalf("expired grace = %v", err)
	}
	if _, _, err := s.Validate(next); err != nil {
		t.Fatalf("successor after grace = %v", err)
	}
}

// A privilege change must rotate on the very next request, even right after a periodic
// rotation. The row keeps one predecessor, so the identifier from two rotations ago is dropped.
func TestPrivilegeRotationInsideGracePeriod(t *testing.T) {
	now := startTime()
	s, _, u := setup(t, &now)
	raw, _, _ := s.Create(u.ID)
	now = now.Add(16 * time.Minute)
	_, next, _ := s.Validate(raw)
	s.MarkNeedsRotation(u.ID)

	_, newest, err := s.Validate(next)
	if err != nil || newest == "" || newest == next {
		t.Fatalf("rotation inside grace = %q, %v", newest, err)
	}
	if _, replay, err := s.Validate(next); err != nil || replay != newest {
		t.Fatalf("previous identifier = %q, %v", replay, err)
	}
	if _, _, err := s.Validate(raw); err != ErrInvalid {
		t.Fatalf("identifier from two rotations ago = %v", err)
	}
	if _, again, err := s.Validate(newest); err != nil || again != "" {
		t.Fatalf("newest identifier = %q, %v", again, err)
	}
}

func TestMarkRotationAndRevoke(t *testing.T) {
	now := startTime()
	s, _, u := setup(t, &now)
	raw, _, _ := s.Create(u.ID)
	s.MarkNeedsRotation(u.ID)
	_, next, err := s.Validate(raw)
	if err != nil || next == "" {
		t.Fatalf("marked rotation = %q, %v", next, err)
	}
	s.Revoke(raw) // the old identifier is still in grace
	if _, _, err := s.Validate(next); err != ErrInvalid {
		t.Fatalf("revoked successor = %v", err)
	}
}

func TestRevokeAllForUserLeavesOthers(t *testing.T) {
	now := startTime()
	s, db, u := setup(t, &now)
	other := testutil.CreateUser(t, db, "budi")
	raw1, _, _ := s.Create(u.ID)
	raw2, _, _ := s.Create(u.ID)
	otherRaw, _, _ := s.Create(other.ID)
	s.RevokeAllForUser(u.ID)
	for _, raw := range []string{raw1, raw2} {
		if _, _, err := s.Validate(raw); err != ErrInvalid {
			t.Fatalf("revoke all = %v", err)
		}
	}
	if _, _, err := s.Validate(otherRaw); err != nil {
		t.Fatalf("another user's session was revoked: %v", err)
	}
}

func TestConcurrentRotationHasOneSuccessor(t *testing.T) {
	now := startTime()
	s, db, u := setup(t, &now)
	raw, _, _ := s.Create(u.ID)
	now = now.Add(16 * time.Minute)

	const workers = 8
	var wg sync.WaitGroup
	results := make(chan string, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, next, err := s.Validate(raw)
			if err != nil {
				t.Error(err)
			}
			results <- next
		}()
	}
	wg.Wait()
	close(results)

	var first string
	for next := range results {
		if next == "" {
			t.Fatal("a request got no replacement identifier")
		}
		if first == "" {
			first = next
		}
		if next != first {
			t.Fatalf("two different successors: %q and %q", first, next)
		}
	}
	want := sha256.Sum256([]byte(first))
	if row := loadRow(t, db, u.ID); !bytes.Equal(row.CurrentHash, want[:]) {
		t.Fatal("stored identifier is not the one every request received")
	}
}

func TestLastSeenIsThrottled(t *testing.T) {
	now := startTime()
	s, db, u := setup(t, &now)
	raw, _, _ := s.Create(u.ID)

	now = startTime().Add(30 * time.Second)
	if _, _, err := s.Validate(raw); err != nil {
		t.Fatal(err)
	}
	if got := loadRow(t, db, u.ID).LastSeen; !got.Equal(startTime()) {
		t.Fatalf("last_seen written %v after the previous write, want no write", got.Sub(startTime()))
	}

	now = startTime().Add(90 * time.Second)
	if _, _, err := s.Validate(raw); err != nil {
		t.Fatal(err)
	}
	if got := loadRow(t, db, u.ID).LastSeen; !got.Equal(now) {
		t.Fatalf("last_seen = %v, want %v", got, now)
	}
}

func TestCleanup(t *testing.T) {
	now := startTime()
	s, db, u := setup(t, &now)
	raw, _, _ := s.Create(u.ID)

	// An expired grace period is cleared; the session itself stays.
	now = now.Add(16 * time.Minute)
	if _, _, err := s.Validate(raw); err != nil {
		t.Fatal(err)
	}
	now = now.Add(31 * time.Second)
	s.Cleanup()
	row := loadRow(t, db, u.ID)
	if row.PreviousHash != nil || row.SuccessorEnc != nil || row.GraceUntil != nil {
		t.Fatal("grace data left behind after the grace period")
	}

	// An idle session is deleted.
	now = now.Add(31 * time.Minute)
	s.Cleanup()
	if n := count(t, db, u.ID); n != 0 {
		t.Fatalf("expired sessions left: %d", n)
	}
}

// The reason sessions live in the database: a new Store (a restarted server) still accepts them.
func TestSessionSurvivesRestart(t *testing.T) {
	now := startTime()
	first, db, u := setup(t, &now)
	raw, created, _ := first.Create(u.ID)

	now = now.Add(16 * time.Minute)
	_, rotated, err := first.Validate(raw)
	if err != nil || rotated == "" {
		t.Fatalf("rotation = %q, %v", rotated, err)
	}
	first.Close()

	restarted := newStore(t, db, &now)
	got, _, err := restarted.Validate(rotated)
	if err != nil || got.UserID != u.ID || got.CSRFToken != created.CSRFToken {
		t.Fatalf("after restart: %#v, %v", got, err)
	}
	// The grace window survives too: the old cookie still gets its replacement.
	if _, replay, err := restarted.Validate(raw); err != nil || replay != rotated {
		t.Fatalf("grace after restart = %q, %v", replay, err)
	}
}

func TestDeletingUserEndsTheirSessions(t *testing.T) {
	now := startTime()
	s, db, u := setup(t, &now)
	raw, _, _ := s.Create(u.ID)
	if err := db.Delete(&models.User{}, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Validate(raw); err != ErrInvalid {
		t.Fatalf("session of a deleted user = %v", err)
	}
	if n := count(t, db, u.ID); n != 0 {
		t.Fatalf("sessions left after user delete: %d", n)
	}
}
