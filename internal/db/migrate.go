package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed migrations
var migrations embed.FS

// Migrations returns the embedded goose migration files.
func Migrations() (fs.FS, error) {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("embedded migrations: %w", err)
	}
	return sub, nil
}

// Migrate applies every pending migration in fsys, each in its own
// transaction. It returns the schema version afterwards and the versions it
// applied. A filesystem with no migrations is not an error.
func Migrate(ctx context.Context, sqlDB *sql.DB, fsys fs.FS) (version int64, applied []int64, err error) {
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, fsys)
	if errors.Is(err, goose.ErrNoMigrations) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, fmt.Errorf("migrate: %w", err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		// On failure goose returns no results, but reports what did get
		// applied before the failing migration in a PartialError.
		var partial *goose.PartialError
		if errors.As(err, &partial) {
			for _, r := range partial.Applied {
				applied = append(applied, r.Source.Version)
			}
		}
		return 0, applied, fmt.Errorf("migrate: %w", err)
	}
	for _, r := range results {
		applied = append(applied, r.Source.Version)
	}

	version, err = provider.GetDBVersion(ctx)
	if err != nil {
		return 0, applied, fmt.Errorf("migrate: read version: %w", err)
	}
	return version, applied, nil
}
