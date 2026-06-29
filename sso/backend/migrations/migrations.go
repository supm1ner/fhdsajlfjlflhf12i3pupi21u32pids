// Package migrations embeds cotton-id's SQL migration files so the binary is
// self-contained. The hand-written runner in internal/database consumes [FS].
package migrations

import "embed"

// FS holds every *.up.sql / *.down.sql migration, embedded at build time.
//
//go:embed *.sql
var FS embed.FS
