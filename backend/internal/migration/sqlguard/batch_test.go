package sqlguard

import "testing"

func TestBatchProblem(t *testing.T) {
	const (
		oneStatement = "SQL per-batch harus tepat satu statement."
		badParam     = "SQL per-batch hanya boleh memakai parameter $1 dan $2."
		missing      = "SQL per-batch harus memakai $1 (batas bawah, tidak termasuk) dan $2 (batas atas, termasuk)."
	)
	cases := []struct {
		name string
		sql  string
		want string
	}{
		{"valid", "UPDATE t SET a = 1 WHERE id > $1 AND id <= $2", ""},
		{"trailing semicolon", "UPDATE t SET a = 1 WHERE id > $1 AND id <= $2;", ""},
		{"parameter in string ignored", "UPDATE t SET a = '$3' WHERE id > $1 AND id <= $2", ""},
		{"parameter in comment ignored", "UPDATE t SET a = 1 /* $3 */ WHERE id > $1 AND id <= $2 -- $4", ""},
		{"trailing comment", "UPDATE t SET a = 1 WHERE id > $1 AND id <= $2; -- done", ""},
		{"trailing block comment", "UPDATE t SET a = 1 WHERE id > $1 AND id <= $2; /* done */", ""},
		{"only a comment", "-- nothing", oneStatement},
		{"empty", "", oneStatement},
		{"two statements", "UPDATE t SET a = 1 WHERE id > $1 AND id <= $2; SELECT 1", oneStatement},
		{"third parameter", "UPDATE t SET a = 1 WHERE id > $1 AND id <= $2 AND b = $3", badParam},
		{"parameter zero", "UPDATE t SET a = $0 WHERE id > $1 AND id <= $2", badParam},
		{"parameter twelve", "UPDATE t SET a = $12 WHERE id > $1 AND id <= $2", badParam},
		{"missing upper", "UPDATE t SET a = 1 WHERE id > $1", missing},
		{"missing lower", "UPDATE t SET a = 1 WHERE id <= $2", missing},
		{"parameter only in string", "UPDATE t SET a = '$1 $2'", missing},
		{"parameter before dollar quote", "UPDATE t SET a = $1$$x$$ WHERE id <= $2", ""},
		{"parameter after identifier byte", "UPDATE t SET a = 1 WHERE id > $1 AND id <= $2 AND x$3 = 1", ""},
	}
	for _, c := range cases {
		if got := BatchProblem(c.sql); got != c.want {
			t.Errorf("%s: BatchProblem(%q) = %q, want %q", c.name, c.sql, got, c.want)
		}
	}
}
