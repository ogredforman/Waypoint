package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" driver; pure Go, no CGO
)

// Open opens (creating if needed) the SQLite database at path and verifies it
// is usable. The pragmas are part of the DSN so every pooled connection gets
// them, not just the first one:
//
//   - journal_mode=WAL and synchronous=NORMAL: concurrent readers, fast commits.
//   - foreign_keys=ON: SQLite leaves these off per connection unless asked.
//   - busy_timeout=5000: wait up to 5s for a lock instead of failing at once.
//
// _txlock=immediate makes BEGIN take the write lock up front, so a transaction
// that reads then writes can't fail with SQLITE_BUSY when it tries to upgrade.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	// The driver splits the DSN at the first '?', so one in the path would be
	// misread as the start of the options.
	if strings.Contains(path, "?") {
		return nil, fmt.Errorf("open %s: database path must not contain '?'", path)
	}
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Set("_txlock", "immediate")

	sqlDB, err := sql.Open("sqlite", path+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := check(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return sqlDB, nil
}

func check(ctx context.Context, sqlDB *sql.DB) error {
	var mode string
	if err := sqlDB.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("read journal_mode: %w", err)
	}
	if mode != "wal" {
		return fmt.Errorf("journal_mode is %q, want \"wal\"", mode)
	}
	return nil
}
