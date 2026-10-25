//go:build integration

package auth_test

import (
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/AthillaZaidan/datanest/backend/internal/auth"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

func TestSeedAdmin(t *testing.T) {
	db := testutil.PlatformDB(t)
	if err := auth.SeedAdmin(db, "", ""); err == nil {
		t.Fatal("started without an admin and without credentials")
	}
	if err := auth.SeedAdmin(db, "root", "first-password"); err != nil {
		t.Fatal(err)
	}
	var u models.User
	if err := db.Where("username = ?", "root").First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if u.PasswordHash == "first-password" || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("first-password")) != nil {
		t.Fatal("password not stored as a bcrypt hash")
	}

	// A second start never resets an existing admin, and no longer needs the credentials.
	if err := auth.SeedAdmin(db, "root", "second-password"); err != nil {
		t.Fatal(err)
	}
	if err := auth.SeedAdmin(db, "", ""); err != nil {
		t.Fatal(err)
	}
	var again models.User
	db.Where("username = ?", "root").First(&again)
	if bcrypt.CompareHashAndPassword([]byte(again.PasswordHash), []byte("first-password")) != nil {
		t.Fatal("existing admin password was reset")
	}
	var admins int64
	db.Model(&models.UserRole{}).Count(&admins)
	if admins != 1 {
		t.Fatalf("admins = %d", admins)
	}
}
