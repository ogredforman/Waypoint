package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

// GetSettings returns the database's settings, or domain.DefaultSettings if
// none are stored yet.
func (r reader) GetSettings(ctx context.Context) (domain.Settings, error) {
	row, err := r.q.GetSettings(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DefaultSettings(), nil
	}
	if err != nil {
		return domain.Settings{}, fmt.Errorf("get settings: %w", err)
	}
	return settingsFromRow(row), nil
}

func settingsFromRow(row dbgen.Setting) domain.Settings {
	return domain.Settings{
		ActiveProjectCap:  int(row.ActiveProjectCap),
		QuestionsPerEntry: int(row.QuestionsPerEntry),
		InterfaceFont:     domain.InterfaceFont(row.InterfaceFont),
		DiaryFont:         domain.DiaryFont(row.DiaryFont),
	}
}

// SeedSettings stores s as the database's settings if it has none yet. Stored
// settings are left as they are, so the config values that seed a new
// database never override what the user chose.
func (s *Store) SeedSettings(ctx context.Context, set domain.Settings) error {
	if err := set.Validate(); err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}
	return s.InTx(ctx, func(tx *Tx) error {
		err := tx.q.InsertSettingsIfMissing(ctx, dbgen.InsertSettingsIfMissingParams{
			ActiveProjectCap:  int64(set.ActiveProjectCap),
			QuestionsPerEntry: int64(set.QuestionsPerEntry),
			InterfaceFont:     string(set.InterfaceFont),
			DiaryFont:         string(set.DiaryFont),
			Now:               tx.nowMS(),
		})
		if err != nil {
			return fmt.Errorf("seed settings: %w", err)
		}
		return nil
	})
}

// UpdateSettings replaces the database's settings with s.
func (tx *Tx) UpdateSettings(ctx context.Context, s domain.Settings) (domain.Settings, error) {
	if err := s.Validate(); err != nil {
		return domain.Settings{}, fmt.Errorf("update settings: %w", err)
	}
	row, err := tx.q.UpsertSettings(ctx, dbgen.UpsertSettingsParams{
		ActiveProjectCap:  int64(s.ActiveProjectCap),
		QuestionsPerEntry: int64(s.QuestionsPerEntry),
		InterfaceFont:     string(s.InterfaceFont),
		DiaryFont:         string(s.DiaryFont),
		Now:               tx.nowMS(),
	})
	if err != nil {
		return domain.Settings{}, fmt.Errorf("update settings: %w", err)
	}
	return settingsFromRow(row), nil
}
