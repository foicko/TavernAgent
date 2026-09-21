//go:build cgo

package sqlite

import (
	_ "github.com/mattn/go-sqlite3"
)

// DriverName is the SQL driver name for native CGO SQLite.
const DriverName = "sqlite3"

// DriverKind identifies the driver implementation.
const DriverKind = "cgo (mattn/go-sqlite3)"
