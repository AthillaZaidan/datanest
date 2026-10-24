package migration

import "sync"

// DBLocks allows one running migration per database; different databases run in parallel.
// It is in-memory, which is enough because the backend runs as a single instance.
type DBLocks struct {
	mu    sync.Mutex
	locks map[uint]*sync.Mutex
}

func NewDBLocks() *DBLocks {
	return &DBLocks{locks: map[uint]*sync.Mutex{}}
}

// TryLock never waits: ok is false when the database already has a running migration.
func (l *DBLocks) TryLock(databaseID uint) (unlock func(), ok bool) {
	l.mu.Lock()
	m, exists := l.locks[databaseID]
	if !exists {
		m = &sync.Mutex{}
		l.locks[databaseID] = m
	}
	l.mu.Unlock()

	if !m.TryLock() {
		return nil, false
	}
	return m.Unlock, true
}
