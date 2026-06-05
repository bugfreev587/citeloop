// Package migrations embeds the forward-only SQL migration files so the binary
// can apply them at startup. The same directory is sqlc's schema source.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
