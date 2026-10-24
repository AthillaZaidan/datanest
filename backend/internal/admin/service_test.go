package admin

import (
	"golang.org/x/crypto/bcrypt"
	"strings"
	"testing"
)

func TestPasswordPolicy(t *testing.T) {
	for _, p := range []string{"short", strings.Repeat("a", 73)} {
		if _, e := hash(p); e == nil {
			t.Fatal("accepted invalid password")
		}
	}
	h, e := hash("password123")
	if e != nil || bcrypt.CompareHashAndPassword([]byte(h), []byte("password123")) != nil {
		t.Fatal("hash failed")
	}
	if h == "password123" {
		t.Fatal("plaintext")
	}
}
func TestName(t *testing.T) {
	for _, n := range []string{"", "   ", strings.Repeat("x", 65)} {
		if validName(n) == nil {
			t.Fatal(n)
		}
	}
	if e := validName("Developers"); e != nil {
		t.Fatal(e)
	}
}
