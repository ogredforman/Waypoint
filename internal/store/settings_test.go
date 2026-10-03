package store

import (
	"context"
	"errors"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestSettings(t *testing.T) {
	ctx := context.Background()
	s, sqlDB, _ := newTestStore(t)

	got, err := s.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != domain.DefaultSettings() {
		t.Errorf("settings before seeding = %+v, want the defaults %+v", got, domain.DefaultSettings())
	}

	seeded := domain.Settings{ActiveProjectCap: 5, QuestionsPerEntry: 1, InterfaceFont: domain.FontGeist, DiaryFont: domain.DiaryLiterata}
	if err := s.SeedSettings(ctx, seeded); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetSettings(ctx); got != seeded {
		t.Errorf("after seeding = %+v, want %+v", got, seeded)
	}

	// A second seed, as at every startup, leaves the stored row alone.
	if err := s.SeedSettings(ctx, domain.DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetSettings(ctx); got != seeded {
		t.Errorf("after a second seed = %+v, want %+v unchanged", got, seeded)
	}

	updated := domain.Settings{ActiveProjectCap: 1, QuestionsPerEntry: 0, InterfaceFont: domain.FontAtkinsonHyperlegibleNext, DiaryFont: domain.DiaryFont(domain.FontIBMPlexSans)}
	inTx(t, s, func(tx *Tx) error {
		got, err := tx.UpdateSettings(ctx, updated)
		if err == nil && got != updated {
			t.Errorf("UpdateSettings returned %+v, want %+v", got, updated)
		}
		return err
	})
	if got, _ := s.GetSettings(ctx); got != updated {
		t.Errorf("after update = %+v, want %+v", got, updated)
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM settings`); n != 1 {
		t.Errorf("%d settings rows, want 1", n)
	}
}

func TestSettingsUpdateWithoutSeed(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newTestStore(t)
	want := domain.Settings{ActiveProjectCap: 4, QuestionsPerEntry: 2, InterfaceFont: domain.FontManrope, DiaryFont: domain.DiaryLiterata}
	inTx(t, s, func(tx *Tx) error {
		_, err := tx.UpdateSettings(ctx, want)
		return err
	})
	if got, _ := s.GetSettings(ctx); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSettingsRejectsInvalid(t *testing.T) {
	ctx := context.Background()
	s, sqlDB, _ := newTestStore(t)
	with := func(change func(*domain.Settings)) domain.Settings {
		s := domain.DefaultSettings()
		change(&s)
		return s
	}
	for _, bad := range []domain.Settings{
		with(func(s *domain.Settings) { s.ActiveProjectCap = 0 }),
		with(func(s *domain.Settings) { s.QuestionsPerEntry = 3 }),
		with(func(s *domain.Settings) { s.QuestionsPerEntry = -1 }),
		with(func(s *domain.Settings) { s.InterfaceFont = "comic-sans" }),
		with(func(s *domain.Settings) { s.InterfaceFont = "literata" }),
		with(func(s *domain.Settings) { s.InterfaceFont = "" }),
		with(func(s *domain.Settings) { s.DiaryFont = "papyrus" }),
	} {
		var ve *domain.ValidationError
		if err := s.SeedSettings(ctx, bad); !errors.As(err, &ve) {
			t.Errorf("SeedSettings(%+v) = %v, want a ValidationError", bad, err)
		}
		err := s.InTx(ctx, func(tx *Tx) error {
			_, err := tx.UpdateSettings(ctx, bad)
			return err
		})
		if !errors.As(err, &ve) {
			t.Errorf("UpdateSettings(%+v) = %v, want a ValidationError", bad, err)
		}
	}
	if n := count(t, sqlDB, `SELECT count(*) FROM settings`); n != 0 {
		t.Errorf("%d settings rows, want 0", n)
	}
}
