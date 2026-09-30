package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
)

// ErrNotFound is returned when the requested row doesn't exist.
var ErrNotFound = errors.New("not found")

// Store reads from the database and starts write transactions.
type Store struct {
	reader
	db  *sql.DB
	now func() time.Time
}

// New returns a Store over a migrated database. now supplies the timestamps
// written to created_at and updated_at.
func New(sqlDB *sql.DB, now func() time.Time) *Store {
	return &Store{reader: reader{q: dbgen.New(sqlDB)}, db: sqlDB, now: now}
}

// Tx is one write transaction. It is only valid inside the InTx callback that
// received it.
type Tx struct {
	reader
	now func() time.Time
}

// reader holds the read methods shared by Store and Tx.
type reader struct {
	q *dbgen.Queries
}

// InTx runs fn in a transaction. It commits if fn returns nil and rolls back
// if fn returns an error or panics; a panic is re-raised after the rollback.
// Inside fn, read through tx, not the Store: the Store uses another
// connection that can't see the transaction's writes. Don't call InTx again.
func (s *Store) InTx(ctx context.Context, fn func(*Tx) error) error {
	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = sqlTx.Rollback()
		}
	}()

	if err := fn(&Tx{reader: reader{q: s.q.WithTx(sqlTx)}, now: s.now}); err != nil {
		return err
	}
	if err := sqlTx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	committed = true
	return nil
}

// ReadTx is one read-only transaction. It is only valid inside the InReadTx
// callback that received it.
type ReadTx struct {
	reader
}

// InReadTx runs fn in a read-only transaction, so its reads see one
// consistent state. Unlike InTx it doesn't take the write lock, so it doesn't
// hold up writers. The transaction is always rolled back; there is nothing to
// commit.
func (s *Store) InReadTx(ctx context.Context, fn func(*ReadTx) error) error {
	// ReadOnly makes the driver issue a plain (deferred) BEGIN instead of the
	// DSN's BEGIN IMMEDIATE.
	sqlTx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin read tx: %w", err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	return fn(&ReadTx{reader: reader{q: s.q.WithTx(sqlTx)}})
}

// nowMS is the transaction's current time in unix milliseconds.
func (tx *Tx) nowMS() int64 { return tx.now().UnixMilli() }

// fromMS converts a stored unix-millisecond timestamp to UTC time.
func fromMS(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// notFound maps sql.ErrNoRows to ErrNotFound and leaves other errors alone.
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Conversions between optional enums and the nullable TEXT columns.

func enumPtrToString[T ~string](v *T) *string {
	if v == nil {
		return nil
	}
	s := string(*v)
	return &s
}

func stringPtrToEnum[T ~string](s *string) *T {
	if s == nil {
		return nil
	}
	v := T(*s)
	return &v
}

func checkLimit(limit int) error {
	if limit <= 0 {
		return fmt.Errorf("limit must be positive, got %d", limit)
	}
	return nil
}
