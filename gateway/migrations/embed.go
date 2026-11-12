// Package migrations holds the SQL schema, applied at startup with golang-migrate.
package migrations

import "embed"

// FS holds the migration files.
//
//go:embed *.sql
var FS embed.FS
