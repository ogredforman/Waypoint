package domain

import (
	"errors"
	"testing"
)

func TestSettingsValidateFonts(t *testing.T) {
	if err := DefaultSettings().Validate(); err != nil {
		t.Fatalf("defaults invalid: %v", err)
	}
	for _, f := range InterfaceFonts() {
		s := DefaultSettings()
		s.InterfaceFont = f
		s.DiaryFont = DiaryFont(f)
		if err := s.Validate(); err != nil {
			t.Errorf("fonts %q rejected: %v", f, err)
		}
	}
	for _, tt := range []struct {
		field  string
		change func(*Settings)
	}{
		{"interface_font", func(s *Settings) { s.InterfaceFont = "literata" }},
		{"interface_font", func(s *Settings) { s.InterfaceFont = "" }},
		{"diary_font", func(s *Settings) { s.DiaryFont = "papyrus" }},
		{"theme", func(s *Settings) { s.Theme = "solarized" }},
		{"theme", func(s *Settings) { s.Theme = "" }},
	} {
		s := DefaultSettings()
		tt.change(&s)
		var ve *ValidationError
		if err := s.Validate(); !errors.As(err, &ve) || len(ve.Problems) != 1 || ve.Problems[0].Field != tt.field {
			t.Errorf("%+v: got %v, want one problem with %s", s, err, tt.field)
		}
	}
}

func TestSettingsValidateThemes(t *testing.T) {
	for _, theme := range Themes() {
		s := DefaultSettings()
		s.Theme = theme
		if err := s.Validate(); err != nil {
			t.Errorf("theme %q rejected: %v", theme, err)
		}
	}
}

func TestCopyAppearance(t *testing.T) {
	base := Settings{ActiveProjectCap: 4, QuestionsPerEntry: 1, Theme: ThemeMidnight, InterfaceFont: FontInter, DiaryFont: DiaryLiterata}
	from := Settings{ActiveProjectCap: 9, QuestionsPerEntry: 0, Theme: ThemeDawn, InterfaceFont: FontGeist, DiaryFont: DiaryFont(FontManrope)}
	want := Settings{ActiveProjectCap: 4, QuestionsPerEntry: 1, Theme: ThemeDawn, InterfaceFont: FontGeist, DiaryFont: DiaryFont(FontManrope)}
	if got := base.CopyAppearance(from); got != want {
		t.Errorf("CopyAppearance = %+v, want %+v", got, want)
	}
}
