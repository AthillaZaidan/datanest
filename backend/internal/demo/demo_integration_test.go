//go:build integration

package demo_test

import (
	"context"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/demo"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

func TestSeedIsIdempotent(t *testing.T) {
	db := testutil.PlatformDB(t)
	cipher, err := dbconn.NewCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := demo.Seed(db, cipher); err != nil {
			t.Fatal(err)
		}
	}
	var n int64
	db.Model(&models.User{}).Count(&n)
	if n != int64(len(demo.Accounts)) {
		t.Fatalf("users = %d", n)
	}
	for _, a := range demo.Accounts {
		var u models.User
		if err := db.Where("username = ?", a.Username).First(&u).Error; err != nil {
			t.Fatal(err)
		}
		if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(a.Password)) != nil {
			t.Fatalf("%s password does not match", a.Username)
		}
	}
	var toko models.DatabaseConnection
	db.Where("name = ?", "toko").First(&toko)
	if p, err := cipher.Decrypt(toko.PasswordEnc); err != nil || p != "platform_app" {
		t.Fatal("connection password", err)
	}
	az := authz.New(db)
	ids := map[string]uint{}
	for _, a := range demo.Accounts {
		var u models.User
		db.Where("username = ?", a.Username).First(&u)
		ids[a.Username] = u.ID
	}
	for user, want := range map[string]authz.Level{"developer": authz.Edit, "reviewer": authz.View, "demo-admin": authz.None} {
		if got, _ := az.EffectiveLevel(context.Background(), ids[user], toko.ID); got != want {
			t.Errorf("%s on toko = %v, want %v", user, got, want)
		}
	}
}
