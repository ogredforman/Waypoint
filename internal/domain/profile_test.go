package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestProfileSlug(t *testing.T) {
	for name, want := range map[string]string{
		"Personal":                "personal",
		"  My Portfolio!  ":       "my-portfolio",
		"Side -- projects 2026":   "side-projects-2026",
		"Ärger & Co":              "rger-co",
		"日記":                      "profile",
		"":                        "profile",
		strings.Repeat("ab ", 30): "ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-a",
	} {
		got := ProfileSlug(name)
		if got != want {
			t.Errorf("ProfileSlug(%q) = %q, want %q", name, got, want)
		}
		if !ValidProfileID(got) || !ValidProfileID(got+"-2") {
			t.Errorf("ProfileSlug(%q) = %q is not a valid ID", name, got)
		}
	}
}

func TestValidProfileID(t *testing.T) {
	for _, id := range []string{"", "-a", "a-", "a--b", "A", "a.b", "../x", "a/b", "a b", "con", "aux", "com3", "lpt1", strings.Repeat("a", 45)} {
		if ValidProfileID(id) {
			t.Errorf("ValidProfileID(%q) = true, want false", id)
		}
	}
}

func TestCleanProfileName(t *testing.T) {
	got, err := CleanProfileName("  Portfolio ")
	if err != nil || got != "Portfolio" {
		t.Errorf("CleanProfileName = %q, %v; want Portfolio, nil", got, err)
	}
	if _, err := CleanProfileName(strings.Repeat("é", ProfileNameMax)); err != nil {
		t.Errorf("a name of exactly %d characters: %v", ProfileNameMax, err)
	}
	for _, bad := range []string{"", "   ", strings.Repeat("x", ProfileNameMax+1)} {
		_, err := CleanProfileName(bad)
		var verr *ValidationError
		if !errors.As(err, &verr) || verr.Problems[0].Field != "name" {
			t.Errorf("CleanProfileName(%q) = %v, want a name problem", bad, err)
		}
	}
}
