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
	} {
		s := DefaultSettings()
		tt.change(&s)
		var ve *ValidationError
		if err := s.Validate(); !errors.As(err, &ve) || len(ve.Problems) != 1 || ve.Problems[0].Field != tt.field {
			t.Errorf("%+v: got %v, want one problem with %s", s, err, tt.field)
		}
	}
}
