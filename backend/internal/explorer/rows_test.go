package explorer

import (
	"encoding/json"
	"testing"
	"time"
)

func TestClampPage(t *testing.T) {
	tests := []struct{ page, size, wantPage, wantSize int }{
		{1, 25, 1, 25},
		{0, 0, 1, DefaultPageSize},
		{-3, -1, 1, DefaultPageSize},
		{2, 10, 2, 10},
		{5, 500, 5, MaxPageSize},
		{1, 100, 1, 100},
	}
	for _, tt := range tests {
		p, s := ClampPage(tt.page, tt.size)
		if p != tt.wantPage || s != tt.wantSize {
			t.Errorf("ClampPage(%d, %d) = %d, %d; want %d, %d", tt.page, tt.size, p, s, tt.wantPage, tt.wantSize)
		}
	}
}

func TestParseInt(t *testing.T) {
	if ParseInt("", 25) != 25 || ParseInt("abc", 1) != 1 || ParseInt("50", 25) != 50 {
		t.Fatal("ParseInt mismatch")
	}
}

func TestJSONValue(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name   string
		dbType string
		in     any
		want   string
	}{
		{"bytea", "BYTEA", []byte{0xde, 0xad}, `"\\xdead"`},
		{"jsonb", "JSONB", []byte(`{"a":1}`), `{"a":1}`},
		{"invalid json falls back to text", "JSON", []byte(`{oops`), `"{oops"`},
		{"text bytes", "TEXT", []byte("halo"), `"halo"`},
		{"int", "INT8", int64(7), `7`},
		{"null", "TEXT", nil, `null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(jsonValue(tt.dbType, tt.in))
			if err != nil || string(b) != tt.want {
				t.Fatalf("got %s, %v; want %s", b, err, tt.want)
			}
		})
	}
	if got := jsonValue("TIMESTAMPTZ", now); got != now {
		t.Fatal("non-byte values must pass through")
	}
}
