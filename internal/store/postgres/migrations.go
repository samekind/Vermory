package postgres

import "embed"

// Migrations is the authoritative schema history embedded into release binaries.
//
//go:embed migrations/*.sql
var Migrations embed.FS
