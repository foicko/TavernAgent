//go:build !cgo

package sqlite

import (
	_ "modernc.org/sqlite"
)

// DriverName is the SQL driver name for pure Go SQLite.
const DriverName = "sqlite"

// DriverKind identifies the driver implementation.
const DriverKind = "pure-go (modernc)"
