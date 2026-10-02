package profile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ogredforman/waypoint/internal/db"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

const (
	// dbExt ends every profile's file name: <id>.db.
	dbExt = ".db"
	// newExt marks a profile still being built (<id>.db.new); it is renamed
	// to <id>.db once ready, so a crash never leaves a half-made profile.
	newExt = ".new"
)

// sidecars are the files SQLite keeps next to a database in WAL mode.
var sidecars = []string{"-wal", "-shm"}

// openMigrated opens the database at path and applies every migration.
func openMigrated(ctx context.Context, path string) (*sql.DB, int64, []int64, error) {
	sqlDB, err := db.Open(ctx, path)
	if err != nil {
		return nil, 0, nil, err
	}
	fsys, err := db.Migrations()
	if err != nil {
		_ = sqlDB.Close()
		return nil, 0, nil, err
	}
	version, applied, err := db.Migrate(ctx, sqlDB, fsys)
	if err != nil {
		_ = sqlDB.Close()
		return nil, 0, nil, err
	}
	return sqlDB, version, applied, nil
}

// prepare brings the database at path up to date, names it name (unless
// name is empty, which keeps the stored one), seeds its settings, and closes
// it with everything written to the main file, so the file can be renamed on
// its own. It returns the profile's name.
func prepare(ctx context.Context, path, name string, seed domain.Settings, now func() time.Time) (string, error) {
	sqlDB, _, _, err := openMigrated(ctx, path)
	if err != nil {
		return "", err
	}
	st := store.New(sqlDB, now)
	err = func() error {
		if name != "" {
			if err := st.InTx(ctx, func(tx *store.Tx) error {
				_, err := tx.SetProfileName(ctx, name)
				return err
			}); err != nil {
				return err
			}
		}
		if err := st.SeedSettings(ctx, seed); err != nil {
			return err
		}
		meta, err := st.GetProfile(ctx)
		name = meta.Name
		return err
	}()
	if err != nil {
		_ = sqlDB.Close()
		return "", err
	}
	if err := checkpointClose(ctx, sqlDB, path); err != nil {
		return "", err
	}
	return name, nil
}

// checkpointClose copies everything in the write-ahead log into the main
// database file and closes it. It fails if the log still holds anything
// afterwards, since then the main file alone isn't the whole database.
func checkpointClose(ctx context.Context, sqlDB *sql.DB, path string) error {
	var busy, logFrames, checkpointed int
	err := sqlDB.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed)
	closeErr := sqlDB.Close()
	switch {
	case err != nil:
		return fmt.Errorf("checkpoint %s: %w", path, err)
	case busy != 0:
		return fmt.Errorf("checkpoint %s: database busy", path)
	case closeErr != nil:
		return fmt.Errorf("close %s: %w", path, closeErr)
	}
	info, err := os.Stat(path + "-wal")
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	case info.Size() > 0:
		return fmt.Errorf("checkpoint %s: write-ahead log not empty after close", path)
	}
	return removeSidecars(path)
}

// removeSidecars removes path's -wal and -shm files, if any.
func removeSidecars(path string) error {
	for _, s := range sidecars {
		if err := os.Remove(path + s); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// removeDB removes the database at path with its -wal and -shm files.
func removeDB(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return removeSidecars(path)
}

// removeUnfinished removes profiles a crash left half-built in dir.
func removeUnfinished(dir string) error {
	matches, err := filepath.Glob(filepath.Join(dir, "*"+dbExt+newExt+"*"))
	if err != nil {
		return err
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// profileFiles returns the IDs of the profile databases in dir, and the
// names of .db files whose names aren't valid IDs (skipped).
func profileFiles(dir string) (ids, skipped []string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, dbExt) {
			continue
		}
		id := strings.TrimSuffix(name, dbExt)
		if !domain.ValidProfileID(id) {
			skipped = append(skipped, name)
			continue
		}
		ids = append(ids, id)
	}
	return ids, skipped, nil
}
