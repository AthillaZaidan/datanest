package dbconn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"testing"
)

func TestCipher(t *testing.T) {
	c, e := NewCipher(make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	a, e := c.Encrypt("secret")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := c.Encrypt("secret")
	if bytes.Equal(a, b) {
		t.Fatal("nonce reused")
	}
	p, e := c.Decrypt(a)
	if e != nil || p != "secret" {
		t.Fatal("round trip")
	}
	other, _ := NewCipher(bytes.Repeat([]byte{1}, 32))
	if _, e = other.Decrypt(a); e == nil {
		t.Fatal("wrong key accepted")
	}
	a[len(a)-1] ^= 1
	if _, e = c.Decrypt(a); e == nil {
		t.Fatal("tamper accepted")
	}
	if _, e = c.Decrypt(nil); e == nil {
		t.Fatal("short data accepted")
	}
	if _, e = NewCipher(make([]byte, 16)); e == nil {
		t.Fatal("short key")
	}
}
func TestValidateConnection(t *testing.T) {
	i := Input{Name: "test", Host: "localhost", Port: 5432, DBName: "test", Username: "user"}
	if e := validate(&i); e != nil {
		t.Fatal(e)
	}
	for _, port := range []int{0, -1, 65536} {
		x := i
		x.Port = port
		if validate(&x) == nil {
			t.Fatal(port)
		}
	}
	i.SSLMode = "disable&password=leak"
	if validate(&i) == nil {
		t.Fatal("ssl injection")
	}
}
func TestSafeErrorsAndView(t *testing.T) {
	e := ConnectionError(context.Background(), errors.New("password=secret"))
	if !errors.Is(e, apperr.ErrTargetDBUnavailable) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = ctx
	b, _ := json.Marshal(view(models.DatabaseConnection{PasswordEnc: []byte("secret")}))
	if bytes.Contains(b, []byte("secret")) || bytes.Contains(b, []byte("passwordEnc")) {
		t.Fatal(string(b))
	}
	ctx2, c := context.WithTimeout(context.Background(), 0)
	defer c()
	if !errors.Is(ConnectionError(ctx2, errors.New("x")), apperr.ErrTargetDBTimeout) {
		t.Fatal("timeout")
	}
}
