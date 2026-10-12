package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ogredforman/waypoint/internal/eval"
)

func TestRunEval(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte("provider = \"anthropic\"\nmodel = \"other\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := func(k string) (string, bool) {
		if k == "WAYPOINT_DATA_DIR" {
			return dir, true
		}
		return "", false
	}
	const fixtures = "../../testdata/entries"
	all, err := eval.LoadFixtures(fixtures)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("mock over every fixture", func(t *testing.T) {
		var out, errOut bytes.Buffer
		// -provider differs from the config's, so its model isn't carried over.
		code := runEval(context.Background(), []string{"-config", cfg, "-provider", "mock", "-dir", fixtures}, env, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit %d: %s", code, errOut.String())
		}
		report := out.String()
		for _, f := range all {
			if !strings.Contains(report, f.Name) {
				t.Errorf("report lacks %s", f.Name)
			}
		}
		if !strings.Contains(report, "waypoint eval: mock,") || !strings.Contains(report, "ops    P") {
			t.Errorf("report:\n%s", report)
		}
	})

	t.Run("filtered", func(t *testing.T) {
		var out, errOut bytes.Buffer
		code := runEval(context.Background(), []string{"-config", cfg, "-provider", "mock", "-dir", fixtures, "-run", all[0].Name}, env, &out, &errOut)
		if code != 0 || !strings.Contains(out.String(), "1 fixtures") {
			t.Errorf("exit %d:\n%s%s", code, out.String(), errOut.String())
		}
	})

	for _, tc := range []struct {
		name string
		args []string
		code int
	}{
		{"unknown provider", []string{"-config", cfg, "-provider", "nope"}, 2},
		{"missing key", []string{"-config", cfg}, 2}, // the configured anthropic needs an API key
		{"no fixtures match", []string{"-config", cfg, "-provider", "mock", "-dir", fixtures, "-run", "zzz"}, 1},
		{"bad fixture dir", []string{"-config", cfg, "-provider", "mock", "-dir", filepath.Join(dir, "missing")}, 1},
		{"stray argument", []string{"extra"}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := runEval(context.Background(), tc.args, env, &out, &errOut); code != tc.code {
				t.Errorf("exit %d, want %d: %s", code, tc.code, errOut.String())
			}
		})
	}
}
