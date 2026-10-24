package session

import (
	"sync"
	"testing"
	"time"
)

func testStore(now *time.Time) *Store {
	return New(Config{IdleTimeout: 30 * time.Minute, AbsoluteTimeout: 8 * time.Hour, RotationInterval: 15 * time.Minute, GracePeriod: 30 * time.Second}, func() time.Time { return *now })
}

func TestCreateAndValidate(t *testing.T) {
	now := time.Now()
	s := testStore(&now)
	defer s.Close()
	raw, created, err := s.Create(7)
	if err != nil {
		t.Fatal(err)
	}
	got, next, err := s.Validate(raw)
	if err != nil || got != created || next != "" || got.CSRFToken == "" {
		t.Fatalf("Validate = %#v, %q, %v", got, next, err)
	}
}
func TestExpiry(t *testing.T) {
	now := time.Now()
	s := testStore(&now)
	defer s.Close()
	raw, _, _ := s.Create(1)
	now = now.Add(31 * time.Minute)
	if _, _, err := s.Validate(raw); err != ErrInvalid {
		t.Fatalf("idle error = %v", err)
	}
	now = time.Now()
	raw, _, _ = s.Create(1)
	now = now.Add(8*time.Hour + time.Second)
	if _, _, err := s.Validate(raw); err != ErrInvalid {
		t.Fatalf("absolute error = %v", err)
	}
}
func TestRotationAndGrace(t *testing.T) {
	now := time.Now()
	s := testStore(&now)
	defer s.Close()
	raw, sess, _ := s.Create(1)
	created := sess.CreatedAt
	now = now.Add(16 * time.Minute)
	_, next, err := s.Validate(raw)
	if err != nil || next == "" {
		t.Fatalf("rotation = %q, %v", next, err)
	}
	if sess.CreatedAt != created {
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
}
func TestMarkRotationAndRevoke(t *testing.T) {
	now := time.Now()
	s := testStore(&now)
	defer s.Close()
	raw, _, _ := s.Create(5)
	s.MarkNeedsRotation(5)
	_, next, err := s.Validate(raw)
	if err != nil || next == "" {
		t.Fatalf("marked rotation = %q, %v", next, err)
	}
	s.Revoke(raw)
	if _, _, err := s.Validate(next); err != ErrInvalid {
		t.Fatalf("revoked successor = %v", err)
	}
	raw1, _, _ := s.Create(5)
	raw2, _, _ := s.Create(5)
	s.RevokeAllForUser(5)
	for _, raw := range []string{raw1, raw2} {
		if _, _, err := s.Validate(raw); err != ErrInvalid {
			t.Fatalf("revoke all = %v", err)
		}
	}
}
func TestConcurrentRotationHasOneSuccessor(t *testing.T) {
	now := time.Now()
	s := testStore(&now)
	defer s.Close()
	raw, _, _ := s.Create(1)
	now = now.Add(16 * time.Minute)
	var wg sync.WaitGroup
	results := make(chan string, 2)
	for range 2 {
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
	var values []string
	for v := range results {
		values = append(values, v)
	}
	if values[0] == "" || values[1] != values[0] {
		t.Fatalf("successors = %q, %q", values[0], values[1])
	}
}
func TestCleanup(t *testing.T) {
	now := time.Now()
	s := testStore(&now)
	defer s.Close()
	raw, _, _ := s.Create(1)
	now = now.Add(31 * time.Minute)
	s.Cleanup()
	if _, _, err := s.Validate(raw); err != ErrInvalid {
		t.Fatalf("cleanup = %v", err)
	}
}
