//go:build integration

package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

func TestLoginLogsNeverContainSecrets(t *testing.T) {
	lines := captureLogs(t)
	env := newAuthEnv(t, defaultSessions())

	if w, _, _ := env.login("ayu", "wrong-password-xyz"); w.Code != 401 {
		t.Fatalf("failed login = %d", w.Code)
	}
	w, sid, csrf := env.login("ayu", testutil.Password)
	if w.Code != 200 || sid == "" || csrf == "" {
		t.Fatalf("login = %d, sid %q csrf %q", w.Code, sid, csrf)
	}
	env.do(authCall{method: "POST", path: "/api/auth/logout", sid: sid, csrf: csrf, origin: appOrigin})

	all := lines()
	raw, _ := json.Marshal(all)
	for name, secret := range map[string]string{"wrong password": "wrong-password-xyz", "password": testutil.Password, "session id": sid, "csrf token": csrf} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("the %s reached the log", name)
		}
	}

	var failed, succeeded int
	for _, l := range all {
		if l[obs.AttrEvent] != obs.EventAuthLogin {
			continue
		}
		switch l[obs.AttrStatus] {
		case "failed":
			failed++
		case "success":
			succeeded++
			if l[obs.AttrActorID] == nil || l[obs.AttrActor] != "ayu" {
				t.Errorf("successful login line = %v", l)
			}
		}
	}
	if failed != 1 || succeeded != 1 {
		t.Errorf("auth.login lines: %d failed, %d success; want 1 and 1", failed, succeeded)
	}
}
