//go:build integration

package testutil

import (
	"context"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

var _ dbconn.TargetDBProvider = StaticProvider{}

func TestPlatformDBFactoriesAndCascade(t *testing.T) {
	db := PlatformDB(t)

	admin := CreateAdmin(t, db, "admin")
	ani := CreateUser(t, db, "ani")
	dev := CreateGroup(t, db, "dev")
	AddMember(t, db, dev.ID, ani.ID)
	conn := CreateDatabaseConnection(t, db, "toko")
	GrantAccess(t, db, dev.ID, conn.ID, authz.Edit)

	var roles int64
	db.Model(&models.UserRole{}).Where("user_id = ?", admin.ID).Count(&roles)
	if roles != 1 {
		t.Fatalf("admin roles = %d, want 1", roles)
	}

	version := 1
	mig := models.Migration{DatabaseID: conn.ID, Version: &version, SQL: "SELECT 1", Status: models.MigrationSuccess,
		ExecutedByID: &ani.ID, ExecutedByUsername: ani.Username, CreatedByUsername: ani.Username}
	mustCreate(t, db, &mig)

	if err := db.Delete(&ani).Error; err != nil {
		t.Fatal(err)
	}
	var members int64
	db.Model(&models.GroupMember{}).Count(&members)
	if members != 0 {
		t.Errorf("membership should cascade on user delete, %d left", members)
	}
	var got models.Migration
	db.First(&got, mig.ID)
	if got.ExecutedByID != nil || got.ExecutedByUsername != "ani" {
		t.Errorf("migration executor = %v %q, want NULL and snapshot kept", got.ExecutedByID, got.ExecutedByUsername)
	}

	if err := db.Delete(&conn).Error; err != nil {
		t.Fatal(err)
	}
	var grants, migs int64
	db.Model(&models.Grant{}).Count(&grants)
	db.Model(&models.Migration{}).Count(&migs)
	if grants != 0 || migs != 0 {
		t.Errorf("grants=%d migrations=%d, want both cascaded", grants, migs)
	}
}

func TestPlatformDBStartsEmpty(t *testing.T) {
	db := PlatformDB(t)
	CreateUser(t, db, "temp")
	db = PlatformDB(t)
	var users int64
	db.Model(&models.User{}).Count(&users)
	if users != 0 {
		t.Fatalf("users = %d after reset, want 0", users)
	}
}

func TestTargetDBAndProvider(t *testing.T) {
	target := TargetDB(t)
	if _, err := target.Exec(`CREATE TABLE products (id int PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}

	pool, err := StaticProvider{DB: target}.Get(context.Background(), 99)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_name = 'products'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("products table count = %d, %v", n, err)
	}

	target = TargetDB(t)
	target.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&n)
	if n != 0 {
		t.Fatalf("public tables after reset = %d, want 0", n)
	}
}
