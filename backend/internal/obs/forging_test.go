package obs

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestUserInputCannotForgeAnotherLogLine(t *testing.T) {
	forged := "ayu\n{\"level\":\"INFO\",\"event\":\"auth.login\",\"actor\":\"admin\",\"status\":\"success\"}\r\x1b[2J"
	var buf bytes.Buffer

	slog.New(NewHandler(&buf, slog.LevelInfo)).Warn(EventAuthLogin, AttrActor, forged, AttrPath, "/api/x\n/api/y", AttrStatus, "failed")

	out := strings.TrimSuffix(buf.String(), "\n")
	if strings.ContainsAny(out, "\n\r\x1b") {
		t.Fatalf("the line contains a raw control character: %q", out)
	}
	var line map[string]any
	if err := json.Unmarshal([]byte(out), &line); err != nil {
		t.Fatalf("not a single JSON object: %v", err)
	}
	if line[AttrActor] != forged || line[AttrStatus] != "failed" || line["level"] != "WARN" {
		t.Errorf("line = %v; want the input kept verbatim as one string value", line)
	}
}
