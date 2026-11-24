//go:build integration

package dblock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

const ttl = 30 * time.Second

func setup(t *testing.T) (*Store, *gorm.DB) {
	t.Helper()
	db := testutil.PlatformDB(t)
	return New(db), db
}

func expire(t *testing.T, db *gorm.DB, databaseID uint) {
	t.Helper()
	err := db.Exec(`UPDATE database_locks SET lease_until = now() - interval '1 second' WHERE database_id = ?`, databaseID).Error
	if err != nil {
		t.Fatal(err)
	}
}

func rowOf(t *testing.T, db *gorm.DB, databaseID uint) models.DatabaseLock {
	t.Helper()
	var row models.DatabaseLock
	if err := db.First(&row, "database_id = ?", databaseID).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func TestAcquireFirstTime(t *testing.T) {
	s, db := setup(t)
	conn := testutil.CreateDatabaseConnection(t, db, "toko")

	lease, ok, err := s.Acquire(context.Background(), conn.ID, 10, "worker-1", ttl)
	if err != nil || !ok {
		t.Fatalf("acquire = ok %v, err %v", ok, err)
	}
	if lease.Token != 1 {
		t.Errorf("token = %d, want 1", lease.Token)
	}
	row := rowOf(t, db, conn.ID)
	if row.Owner != "worker-1" || row.ExecutionID != 10 || !row.LeaseUntil.After(time.Now()) {
		t.Errorf("row = %+v", row)
	}
}

func TestAcquireBusyIsNotAnError(t *testing.T) {
	s, db := setup(t)
	conn := testutil.CreateDatabaseConnection(t, db, "toko")
	ctx := context.Background()

	if _, ok, _ := s.Acquire(ctx, conn.ID, 10, "worker-1", ttl); !ok {
		t.Fatal("first acquire should succeed")
	}
	_, ok, err := s.Acquire(ctx, conn.ID, 11, "worker-2", ttl)
	if err != nil || ok {
		t.Fatalf("second acquire = ok %v, err %v; want busy without error", ok, err)
	}
	if row := rowOf(t, db, conn.ID); row.Owner != "worker-1" || row.ExecutionID != 10 || row.Token != 1 {
		t.Errorf("busy acquire changed the row: %+v", row)
	}
}

func TestConcurrentAcquireHasOneWinner(t *testing.T) {
	s, db := setup(t)
	conn := testutil.CreateDatabaseConnection(t, db, "toko")

	var wg sync.WaitGroup
	var winners atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok, err := s.Acquire(context.Background(), conn.ID, uint(100+i), "worker", ttl)
			if err != nil {
				t.Error(err)
				return
			}
			if ok {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("%d winners, want 1", winners.Load())
	}
}

func TestTakeoverAfterExpiryRaisesToken(t *testing.T) {
	s, db := setup(t)
	conn := testutil.CreateDatabaseConnection(t, db, "toko")
	ctx := context.Background()

	if _, ok, _ := s.Acquire(ctx, conn.ID, 10, "worker-1", ttl); !ok {
		t.Fatal("first acquire should succeed")
	}
	expire(t, db, conn.ID)

	lease, ok, err := s.Acquire(ctx, conn.ID, 11, "worker-2", ttl)
	if err != nil || !ok {
		t.Fatalf("takeover = ok %v, err %v", ok, err)
	}
	if lease.Token != 2 {
		t.Errorf("token = %d, want 2", lease.Token)
	}
	if row := rowOf(t, db, conn.ID); row.Owner != "worker-2" || row.ExecutionID != 11 {
		t.Errorf("row = %+v", row)
	}
}

func TestRenewExtendsTheLease(t *testing.T) {
	s, db := setup(t)
	conn := testutil.CreateDatabaseConnection(t, db, "toko")
	ctx := context.Background()

	lease, _, _ := s.Acquire(ctx, conn.ID, 10, "worker-1", time.Second)
	before := rowOf(t, db, conn.ID).LeaseUntil

	ok, err := s.Renew(ctx, lease, ttl)
	if err != nil || !ok {
		t.Fatalf("renew = ok %v, err %v", ok, err)
	}
	if after := rowOf(t, db, conn.ID).LeaseUntil; !after.After(before) {
		t.Errorf("lease_until did not move forward: %v then %v", before, after)
	}
}

func TestStaleHolderCannotRenewOrRelease(t *testing.T) {
	s, db := setup(t)
	conn := testutil.CreateDatabaseConnection(t, db, "toko")
	ctx := context.Background()

	old, _, _ := s.Acquire(ctx, conn.ID, 10, "worker-1", ttl)
	expire(t, db, conn.ID)
	current, ok, _ := s.Acquire(ctx, conn.ID, 11, "worker-2", ttl)
	if !ok {
		t.Fatal("takeover should succeed")
	}

	if renewed, err := s.Renew(ctx, old, ttl); err != nil || renewed {
		t.Fatalf("zombie renew = %v, err %v; want false", renewed, err)
	}
	if err := s.Release(ctx, old); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Acquire(ctx, conn.ID, 12, "worker-3", ttl); ok {
		t.Fatal("zombie release freed the lock of the current holder")
	}
	if renewed, _ := s.Renew(ctx, current, ttl); !renewed {
		t.Error("current holder lost its lease")
	}
}

func TestReleaseThenAcquireRaisesToken(t *testing.T) {
	s, db := setup(t)
	conn := testutil.CreateDatabaseConnection(t, db, "toko")
	ctx := context.Background()

	first, _, _ := s.Acquire(ctx, conn.ID, 10, "worker-1", ttl)
	if err := s.Release(ctx, first); err != nil {
		t.Fatal(err)
	}
	second, ok, err := s.Acquire(ctx, conn.ID, 11, "worker-1", ttl)
	if err != nil || !ok {
		t.Fatalf("acquire after release = ok %v, err %v", ok, err)
	}
	if second.Token != first.Token+1 {
		t.Errorf("token = %d, want %d", second.Token, first.Token+1)
	}
	var rows int64
	db.Model(&models.DatabaseLock{}).Where("database_id = ?", conn.ID).Count(&rows)
	if rows != 1 {
		t.Errorf("%d rows for one database, want 1", rows)
	}
}

func TestDifferentDatabasesLockIndependently(t *testing.T) {
	s, db := setup(t)
	a := testutil.CreateDatabaseConnection(t, db, "toko")
	b := testutil.CreateDatabaseConnection(t, db, "kampus")
	ctx := context.Background()

	if _, ok, _ := s.Acquire(ctx, a.ID, 10, "worker-1", ttl); !ok {
		t.Fatal("first database should lock")
	}
	if _, ok, _ := s.Acquire(ctx, b.ID, 11, "worker-2", ttl); !ok {
		t.Fatal("second database should lock while the first is held")
	}
}

func TestDeletingTheConnectionRemovesItsLock(t *testing.T) {
	s, db := setup(t)
	conn := testutil.CreateDatabaseConnection(t, db, "toko")

	if _, ok, _ := s.Acquire(context.Background(), conn.ID, 10, "worker-1", ttl); !ok {
		t.Fatal("acquire should succeed")
	}
	if err := db.Delete(&conn).Error; err != nil {
		t.Fatal(err)
	}
	var rows int64
	db.Model(&models.DatabaseLock{}).Count(&rows)
	if rows != 0 {
		t.Errorf("%d lock rows left after deleting the connection", rows)
	}
}
