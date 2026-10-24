// Package sqlguard rejects migration SQL that would break the platform's transaction.
package sqlguard

// Check returns apperr.ErrTransactionControlNotAllowed when sql contains BEGIN, COMMIT,
// ROLLBACK and similar statements outside strings, comments, and dollar quotes.
//
// TODO(A): implement the tokenizer (PRD §13). Until then every statement passes.
func Check(sql string) error {
	return nil
}
