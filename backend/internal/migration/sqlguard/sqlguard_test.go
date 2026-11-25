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

func TestCheckRejectsHiddenTransactionControl(t *testing.T) {
	for _, sql := range []string{
		"SELECT E'\\''; COMMIT; SELECT 'x'",
		"SELECT e'\\\\\\''; COMMIT; --'",
		"SELECT E'a'\n'\\''; COMMIT; SELECT 'x'",
		"SELECT 1 AS a$b$; COMMIT; SELECT 2 AS c$b$",
		"-- x\rCOMMIT",
		"SELECT 1 AS é$b$; COMMIT; SELECT 2 AS é$b$",
		"SELECT E'a' -- c\n'\\''; COMMIT; SELECT 'x'",
		"SELECT E'a'\r'\\''; COMMIT; SELECT 'x'",
		"SELECT 1$$'$$; COMMIT; --'",
		"SELECT $1$$'$$; COMMIT; --'",
		"SELECT 1E'\\''; COMMIT; SELECT 'x'",
		"SELECT $1E'\\''; COMMIT; SELECT 'x'",
		"SELECT 1.5e3$$'$$; COMMIT; --'",
		"SELECT $1$; COMMIT; $1$",
	} {
		if Check(sql) == nil {
			t.Errorf("Check(%q) accepted transaction control", sql)
		}
	}
}

func TestCheckAllowsLikePostgreSQL(t *testing.T) {
	for _, sql := range []string{
		"SELECT 'it''s; COMMIT'",
		"SELECT E'it\\'s; COMMIT'",
		"SELECT date'2020-01-01'; SELECT 'a'",
		"SELECT '\\'; SELECT 1",
		"CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $body$ BEGIN COMMIT; END $body$",
		"/* a /* COMMIT; */ */ SELECT 1",
		"SELECT \"a;COMMIT\"",
		"SELECT E'a' 'b; COMMIT'",
		"SELECT E'a'\n'b\\'; COMMIT'",
		"SELECT E'a' -- c\n'b\\'; COMMIT'; SELECT 1",
		"SELECT E'a'\r'b\\'; COMMIT'; SELECT 1",
		"SELECT 1 AS $a$; COMMIT; $a$",
		"SELECT E'a' /* c */ \n'\\''; COMMIT; SELECT 'x'",
		"SELECT E'a' 'b'; SELECT 1",
	} {
		if err := Check(sql); err != nil {
			t.Errorf("Check(%q) = %v", sql, err)
		}
	}
}

func TestMaskLiteralsFollowsEscapeStrings(t *testing.T) {
	got := MaskLiterals("SELECT E'\\'' , 'topsecret'")
	if got != "SELECT E'?' , '?'" {
		t.Errorf("MaskLiterals = %q", got)
	}
}

func TestMaskLiteralsMasksAContinuedEscapeString(t *testing.T) {
	got := MaskLiterals("SELECT E'a'\n'\\'secret' , 'x'")
	if got != "SELECT E'?' , '?'" {
		t.Errorf("MaskLiterals = %q", got)
	}
}

func TestMaskLiteralsKeepsParametersAndMasksDollarQuotes(t *testing.T) {
	got := MaskLiterals("SELECT $1$$secret$$, $$x$$ -- c")
	if got != "SELECT $1'?', '?'  " {
		t.Errorf("MaskLiterals = %q", got)
	}
}
