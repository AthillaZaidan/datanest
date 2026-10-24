//go:build integration

package dbconn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
	"github.com/jackc/pgx/v5"
	"os"
	"strconv"
	"testing"
	"time"
)

func targetInput(t *testing.T) Input {
	t.Helper()
	dsn := os.Getenv("TEST_TARGET_DSN")
	if dsn == "" {
		t.Skip("TEST_TARGET_DSN not set")
	}
	cfg, e := pgx.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	return Input{Name: "target", Host: cfg.Host, Port: int(cfg.Port), DBName: cfg.Database, Username: cfg.User, Password: cfg.Password, SSLMode: "disable"}
}
func TestConnectionsAndAccess(t *testing.T) {
	db := testutil.PlatformDB(t)
	ctx := context.Background()
	c, _ := NewCipher(make([]byte, 32))
	p := NewPools(db, c, 200*time.Millisecond)
	defer p.Close()
	s := New(db, c, p, authz.New(db), 200*time.Millisecond)
	i := targetInput(t)
	v, e := s.Create(ctx, i)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Create(ctx, i); !errors.Is(e, apperr.ErrConflict) {
		t.Fatal("duplicate", e)
	}
	var raw models.DatabaseConnection
	db.First(&raw, v.ID)
	secret, e := c.Decrypt(raw.PasswordEnc)
	if e != nil || secret != i.Password {
		t.Fatal("encrypted credential", e)
	}
	pool, e := p.Get(ctx, v.ID)
	if e != nil {
		t.Fatal(e)
	}
	same, e := p.Get(ctx, v.ID)
	if e != nil || same != pool || pool.Stats().MaxOpenConnections != 5 {
		t.Fatal("pool cache")
	}
	result, e := s.TestSaved(ctx, v.ID)
	if e != nil || !result.OK {
		t.Fatal("test saved", e)
	}
	u := testutil.CreateUser(t, db, "user")
	a := testutil.CreateAdmin(t, db, "admin")
	if _, e = s.GetForUser(ctx, a.ID, v.ID); !errors.Is(e, apperr.ErrForbidden) {
		t.Fatal("admin bypass", e)
	}
	list, e := s.ListForUser(ctx, u.ID)
	if e != nil || len(list) != 0 {
		t.Fatal(list, e)
	}
	g := testutil.CreateGroup(t, db, "view")
	g2 := testutil.CreateGroup(t, db, "edit")
	testutil.AddMember(t, db, g.ID, u.ID)
	testutil.AddMember(t, db, g2.ID, u.ID)
	testutil.GrantAccess(t, db, g.ID, v.ID, authz.View)
	testutil.GrantAccess(t, db, g2.ID, v.ID, authz.Edit)
	list, e = s.ListForUser(ctx, u.ID)
	if e != nil || len(list) != 1 || list[0].MyLevel != "EDIT" || list[0].Name != i.Name || list[0].CurrentVersion != 0 {
		t.Fatalf("list %+v %v", list, e)
	}
	for n, status := range []models.MigrationStatus{models.MigrationSuccess, models.MigrationSuccess, models.MigrationFailed} {
		version := n + 1
		if e = db.Create(&models.Migration{DatabaseID: v.ID, Version: &version, SQL: "select 1", Status: status, CreatedByUsername: "user"}).Error; e != nil {
			t.Fatal(e)
		}
	}
	if list, e = s.ListForUser(ctx, u.ID); e != nil || list[0].CurrentVersion != 2 {
		t.Fatalf("current version %+v %v", list, e)
	}
	detail, e := s.GetForUser(ctx, u.ID, v.ID)
	if e != nil || detail.MyLevel != "EDIT" {
		t.Fatal(detail, e)
	}
	data, _ := json.Marshal(detail)
	if bytes.Contains(data, []byte("passwordEnc")) || bytes.Contains(data, []byte(`"password":`)) {
		t.Fatal(string(data))
	}
	empty := ""
	name := "renamed"
	if _, e = s.Update(ctx, v.ID, Patch{Name: &name, Password: &empty}); e != nil {
		t.Fatal(e)
	}
	var updated models.DatabaseConnection
	db.First(&updated, v.ID)
	if !bytes.Equal(raw.PasswordEnc, updated.PasswordEnc) {
		t.Fatal("blank password changed")
	}
	if pool.Ping() == nil {
		t.Fatal("old pool not closed")
	}
	pool2, e := p.Get(ctx, v.ID)
	if e != nil || pool2 == pool {
		t.Fatal("pool not renewed", e)
	}
	password := "new-secret"
	if _, e = s.Update(ctx, v.ID, Patch{Password: &password}); e != nil {
		t.Fatal(e)
	}
	db.First(&updated, v.ID)
	if plain, e := c.Decrypt(updated.PasswordEnc); e != nil || plain != password {
		t.Fatal("password replacement", e)
	}
	version := 1
	m := models.Migration{DatabaseID: v.ID, Version: &version, SQL: "SELECT 1", CreatedByUsername: "user", Status: models.MigrationSuccess}
	db.Create(&m)
	if e = s.Delete(ctx, v.ID); e != nil {
		t.Fatal(e)
	}
	var n int64
	db.Model(&models.Migration{}).Where("id = ?", m.ID).Count(&n)
	if n != 0 {
		t.Fatal("history not cascaded")
	}
	db.Model(&models.Grant{}).Where("database_id = ?", v.ID).Count(&n)
	if n != 0 {
		t.Fatal("grants not cascaded")
	}
	i.Port = 1
	i.Host = "127.0.0.1"
	start := time.Now()
	_, e = s.Test(ctx, i)
	if !errors.Is(e, apperr.ErrTargetDBUnavailable) && !errors.Is(e, apperr.ErrTargetDBTimeout) {
		t.Fatal("connection error", e)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("timeout not bounded")
	}
}
func TestSuperuserAndBadCredentials(t *testing.T) {
	db := testutil.PlatformDB(t)
	c, _ := NewCipher(make([]byte, 32))
	p := NewPools(db, c, time.Second)
	defer p.Close()
	s := New(db, c, p, authz.New(db), time.Second)
	i := targetInput(t)
	ctx := context.Background()
	result, e := s.Test(ctx, i)
	if e != nil {
		t.Fatal(e)
	}
	cfg, _ := pgx.ParseConfig(os.Getenv("TEST_TARGET_DSN"))
	conn, e := pgx.ConnectConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close(ctx)
	var super bool
	if e = conn.QueryRow(ctx, "SELECT rolsuper FROM pg_roles WHERE rolname=current_user").Scan(&super); e != nil {
		t.Fatal(e)
	}
	if result.IsSuperuser != super || (super && result.Warning == "") {
		t.Fatal("missing warning")
	}
	// A nonexistent role always fails, even on trust-authenticated local test clusters.
	i.Username = "nonexistent_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	i.Password = "never-expose-this"
	_, e = s.Test(ctx, i)
	if !errors.Is(e, apperr.ErrTargetDBUnavailable) {
		t.Fatal(e)
	}
	_, body := apperr.Response(e)
	b, _ := json.Marshal(body)
	if bytes.Contains(b, []byte(i.Password)) || bytes.Contains(b, []byte(i.Username)) {
		t.Fatal("credential leak")
	}
}
