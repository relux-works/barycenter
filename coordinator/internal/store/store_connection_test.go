package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"testing"
)

func TestReplacementConnectionRetainsRequiredPragmas(t *testing.T) {
	st, err := OpenWithOptions(
		filepath.Join(t.TempDir(), "replacement-pragmas.db"),
		Options{SelfServiceOnboarding: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	assertConnectionPragmas := func(label string) {
		t.Helper()
		var busyTimeout, foreignKeys int
		if err := st.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
			t.Fatalf("%s busy_timeout: %v", label, err)
		}
		if err := st.db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
			t.Fatalf("%s foreign_keys: %v", label, err)
		}
		if busyTimeout != 5000 || foreignKeys != 1 {
			t.Fatalf("%s pragmas busy_timeout=%d foreign_keys=%d", label, busyTimeout, foreignKeys)
		}
	}

	assertConnectionPragmas("initial connection")
	conn, err := st.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Raw(func(any) error { return driver.ErrBadConn }); !errors.Is(err, driver.ErrBadConn) {
		conn.Close()
		t.Fatalf("discard connection: %v", err)
	}
	if err := conn.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
		t.Fatal(err)
	}

	// SetMaxOpenConns(1) and ErrBadConn above force this query to create a new
	// physical driver connection. These values therefore come from DSN hooks,
	// not from the one-time startup Exec calls.
	assertConnectionPragmas("replacement connection")
}
