package sqlguard

import "testing"

func TestCheckRejectsTransactionControl(t *testing.T) {
	for _, sql := range []string{"BEGIN; SELECT 1", "select 1; COMMIT", "start transaction", "ROLLBACK", "SAVEPOINT s", "RELEASE SAVEPOINT s", "PREPARE TRANSACTION 'x'", "abort"} {
		if Check(sql) == nil {
			t.Errorf("Check(%q) accepted transaction control", sql)
		}
	}
}
func TestCheckAllowsProtectedWords(t *testing.T) {
	for _, sql := range []string{"select 'COMMIT'", "-- ROLLBACK\nselect 1", "/* BEGIN */ select 1", "select \"END\" from t", "create function f() returns void language plpgsql as $$ begin perform 1; end $$;"} {
		if err := Check(sql); err != nil {
			t.Errorf("Check(%q) = %v", sql, err)
		}
	}
}
