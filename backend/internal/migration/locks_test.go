package migration

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestTryLockSameDatabase(t *testing.T) {
	l := NewDBLocks()
	unlock, ok := l.TryLock(1)
	if !ok {
		t.Fatal("first lock should succeed")
	}
	if _, ok := l.TryLock(1); ok {
		t.Fatal("second lock on the same database should fail")
	}
	unlock()
	unlock2, ok := l.TryLock(1)
	if !ok {
		t.Fatal("lock should be available again after unlock")
	}
	unlock2()
}

func TestTryLockDifferentDatabases(t *testing.T) {
	l := NewDBLocks()
	u1, ok1 := l.TryLock(1)
	u2, ok2 := l.TryLock(2)
	if !ok1 || !ok2 {
		t.Fatal("different databases must lock independently")
	}
	u1()
	u2()
}

func TestTryLockConcurrentWhileHeld(t *testing.T) {
	l := NewDBLocks()
	unlock, _ := l.TryLock(7)
	defer unlock()

	var wg sync.WaitGroup
	var winners atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := l.TryLock(7); ok {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 0 {
		t.Fatalf("%d goroutines got a held lock", winners.Load())
	}
}
