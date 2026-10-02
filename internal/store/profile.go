package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

// ProfileMeta is this database's profile row (ADR 0005).
type ProfileMeta struct {
	Name      string
	CreatedAt time.Time
}

// GetProfile returns this database's profile display name and creation time.
func (r reader) GetProfile(ctx context.Context) (ProfileMeta, error) {
	row, err := r.q.GetProfile(ctx)
	if err != nil {
		return ProfileMeta{}, fmt.Errorf("get profile: %w", notFound(err))
	}
	return ProfileMeta{Name: row.Name, CreatedAt: fromMS(row.CreatedAt)}, nil
}

// SetProfileName changes this database's profile display name and returns it
// as stored (trimmed). Uniqueness across profiles is the caller's to check.
func (tx *Tx) SetProfileName(ctx context.Context, name string) (string, error) {
	name, err := domain.CleanProfileName(name)
	if err != nil {
		return "", fmt.Errorf("set profile name: %w", err)
	}
	row, err := tx.q.SetProfileName(ctx, dbgen.SetProfileNameParams{Name: name, Now: tx.nowMS()})
	if err != nil {
		return "", fmt.Errorf("set profile name: %w", notFound(err))
	}
	return row.Name, nil
}
