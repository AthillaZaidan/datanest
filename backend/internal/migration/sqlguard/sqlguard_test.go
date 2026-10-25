package sqlguard

import "testing"

func TestCheckRejectsTransactionControl(t *testing.T) {
	for _, sql := range []string{"BEGIN; SELECT 1", "select 1; COMMIT", "start transaction", "ROLLBACK", "SAVEPOINT s", "RELEASE SAVEPOINT s", "PREPARE TRANSACTION 'x'", "abort", "select 1;\n  end;", "-- x\nbegin"} {
		if Check(sql) == nil {
			t.Errorf("Check(%q) accepted transaction control", sql)
		}
	}
}
func TestCheckAllowsProtectedWords(t *testing.T) {
	for _, sql := range []string{"select 'COMMIT'", "-- ROLLBACK\nselect 1", "/* BEGIN */ select 1", "select \"END\" from t", "create function f() returns void language plpgsql as $$ begin perform 1; end $$;", "create view v as select case when x > 0 then 1 else 0 end as ok from t", "select commit_id, start_date from t", "prepare q as select 1"} {
		if err := Check(sql); err != nil {
			t.Errorf("Check(%q) = %v", sql, err)
		}
	}
}

func TestSplitStatements(t *testing.T) {
	sql := "CREATE TABLE t (a text DEFAULT ';');\n-- skip; this\nINSERT INTO t VALUES ('x;y');\n" +
		"CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END $$ LANGUAGE plpgsql;\n  ;\nSELECT * FROM t"
	got := Split(sql)
	want := []string{
		"CREATE TABLE t (a text DEFAULT ';')",
		"-- skip; this\nINSERT INTO t VALUES ('x;y')",
		"CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END $$ LANGUAGE plpgsql",
		"SELECT * FROM t",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d statements: %q", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("statement %d = %q, want %q", i, got[i], want[i])
		}
	}
}
