package store

import (
	"context"
	"errors"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestProfileName(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()

	// The migration names every database "Personal" (ADR 0005).
	if got, err := s.GetProfile(ctx); err != nil || got.Name != "Personal" || got.CreatedAt.IsZero() {
		t.Fatalf("GetProfile = %+v, %v; want Personal with a creation time", got, err)
	}
	inTx(t, s, func(tx *Tx) error {
		got, err := tx.SetProfileName(ctx, "  Portfolio ")
		if err == nil && got != "Portfolio" {
			t.Errorf("SetProfileName = %q, want Portfolio", got)
		}
		return err
	})
	if got, err := s.GetProfile(ctx); err != nil || got.Name != "Portfolio" {
		t.Errorf("after rename: %+v, %v; want Portfolio", got, err)
	}

	err := s.InTx(ctx, func(tx *Tx) error {
		_, err := tx.SetProfileName(ctx, "   ")
		return err
	})
	var verr *domain.ValidationError
	if !errors.As(err, &verr) {
		t.Errorf("blank name: %v, want a validation error", err)
	}
}
