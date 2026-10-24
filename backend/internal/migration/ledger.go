// Package migration manages drafts and executes migrations on target databases.
package migration

// LedgerTable lives in each target's public schema. It is written in the same transaction
// as the migration, so a row exists exactly when that migration committed.
const LedgerTable = "_platform_migrations"
