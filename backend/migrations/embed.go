// Package migrations embeds the SQL schema migrations so they travel inside
// the binary. The API image is distroless — no shell, no psql — so applying
// them at release time has to be a Go program that already holds the files.
package migrations

import "embed"

// FS holds every migration, named so lexical order is apply order.
//
//go:embed *.sql
var FS embed.FS
