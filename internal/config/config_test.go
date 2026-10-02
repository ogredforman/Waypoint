package config

import (
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeEnv(m map[string]string) LookupEnv {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

// testEnv returns an environment whose default dirs (goos "linux") point into
// a temp dir, plus that config dir. extra entries are added on top.
func testEnv(t *testing.T, extra map[string]string) (LookupEnv, string) {
	t.Helper()
	root := t.TempDir()
	m := map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(root, "config"),
		"XDG_DATA_HOME":   filepath.Join(root, "data"),
	}
	for k, v := range extra {
		m[k] = v
	}
	return fakeEnv(m), filepath.Join(root, "config", "waypoint")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func abs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestLoadDefaults(t *testing.T) {
	env, _ := testEnv(t, nil)
	cfg, err := Load(nil, env, "linux", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	dataHome, _ := env("XDG_DATA_HOME")
	want := Config{Port: DefaultPort, DataDir: filepath.Join(dataHome, "waypoint"), LogLevel: slog.LevelInfo, ActiveProjectCap: 3, QuestionsPerEntry: 2, SnapshotRecentEntries: 5}
	if cfg != want {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
	if got := cfg.Addr(); got != "127.0.0.1:7420" {
		t.Errorf("Addr() = %q", got)
	}
	if got, want := cfg.DBPath(), filepath.Join(want.DataDir, "waypoint.db"); got != want {
		t.Errorf("DBPath() = %q, want %q", got, want)
	}
}

func TestLoadPrecedence(t *testing.T) {
	fileDir := filepath.Join(t.TempDir(), "fromfile")
	file := "port = 1001\ndata_dir = '" + filepath.ToSlash(fileDir) + "'\nlog_level = 'warn'\n"

	tests := []struct {
		name      string
		file      string // written to the default config path if non-empty
		env       map[string]string
		args      []string
		wantPort  int
		wantDir   string // "" means the default data dir
		wantLevel slog.Level
		wantCap   int // 0 means the default
		wantMock  time.Duration
		// wantRecent is SnapshotRecentEntries; nil means the default. 0 is a valid setting.
		wantRecent *int
		// wantQuestions is QuestionsPerEntry; nil means the default. 0 is a valid setting.
		wantQuestions *int
	}{
		{
			name:          "file sets questions per entry",
			file:          "questions_per_entry = 0\n",
			wantPort:      DefaultPort,
			wantLevel:     slog.LevelInfo,
			wantQuestions: ptr(0),
		},
		{
			name:      "file overrides defaults",
			file:      file + "active_project_cap = 5\nmock_latency = '1500ms'\n",
			wantPort:  1001,
			wantDir:   fileDir,
			wantLevel: slog.LevelWarn,
			wantCap:   5,
			wantMock:  1500 * time.Millisecond,
		},
		{
			name:       "file sets snapshot recent entries",
			file:       "snapshot_recent_entries = 2\n",
			wantPort:   DefaultPort,
			wantLevel:  slog.LevelInfo,
			wantRecent: ptr(2),
		},
		{
			name:       "file turns snapshot recent entries off",
			file:       "snapshot_recent_entries = 0\n",
			wantPort:   DefaultPort,
			wantLevel:  slog.LevelInfo,
			wantRecent: ptr(0),
		},
		{
			name:      "env overrides file",
			file:      file,
			env:       map[string]string{"WAYPOINT_PORT": "2002", "WAYPOINT_DATA_DIR": "envdir", "WAYPOINT_LOG_LEVEL": "debug"},
			wantPort:  2002,
			wantDir:   abs(t, "envdir"),
			wantLevel: slog.LevelDebug,
		},
		{
			name:      "flags override env",
			file:      file,
			env:       map[string]string{"WAYPOINT_PORT": "2002", "WAYPOINT_DATA_DIR": "envdir", "WAYPOINT_LOG_LEVEL": "debug"},
			args:      []string{"-port", "3003", "-data-dir", "flagdir", "-log-level", "error"},
			wantPort:  3003,
			wantDir:   abs(t, "flagdir"),
			wantLevel: slog.LevelError,
		},
		{
			name:      "each setting resolved independently",
			file:      file,
			env:       map[string]string{"WAYPOINT_LOG_LEVEL": "debug"},
			args:      []string{"-port", "3003"},
			wantPort:  3003,
			wantDir:   fileDir,
			wantLevel: slog.LevelDebug,
		},
		{
			name:      "empty env var is ignored",
			env:       map[string]string{"WAYPOINT_PORT": ""},
			wantPort:  DefaultPort,
			wantLevel: slog.LevelInfo,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, configDir := testEnv(t, tt.env)
			wantFile := ""
			if tt.file != "" {
				wantFile = filepath.Join(configDir, "config.toml")
				writeFile(t, wantFile, tt.file)
			}
			cfg, err := Load(tt.args, env, "linux", io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			wantCap := tt.wantCap
			if wantCap == 0 {
				wantCap = 3
			}
			wantRecent := DefaultSnapshotRecentEntries
			if tt.wantRecent != nil {
				wantRecent = *tt.wantRecent
			}
			wantQuestions := 2
			if tt.wantQuestions != nil {
				wantQuestions = *tt.wantQuestions
			}
			wantDir := tt.wantDir
			if wantDir == "" {
				dataHome, _ := env("XDG_DATA_HOME")
				wantDir = filepath.Join(dataHome, "waypoint")
			}
			want := Config{Port: tt.wantPort, DataDir: wantDir, LogLevel: tt.wantLevel, ActiveProjectCap: wantCap, QuestionsPerEntry: wantQuestions, MockLatency: tt.wantMock, SnapshotRecentEntries: wantRecent, File: wantFile}
			if cfg != want {
				t.Errorf("got %+v, want %+v", cfg, want)
			}
		})
	}
}

func TestLoadExplicitConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.toml")
	writeFile(t, path, "port = 4004\ndata_dir = 'rel'\n")

	for _, tt := range []struct {
		name string
		env  map[string]string
		args []string
	}{
		{name: "flag", args: []string{"-config", path}},
		{name: "env", env: map[string]string{"WAYPOINT_CONFIG": path}},
		{name: "flag beats env", env: map[string]string{"WAYPOINT_CONFIG": filepath.Join(dir, "missing.toml")}, args: []string{"-config", path}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env, _ := testEnv(t, tt.env)
			cfg, err := Load(tt.args, env, "linux", io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Port != 4004 || cfg.File != path {
				t.Errorf("got port %d file %q, want 4004 %q", cfg.Port, cfg.File, path)
			}
			// A relative data_dir in the file is relative to the file, not the working dir.
			if want := filepath.Join(dir, "rel"); cfg.DataDir != want {
				t.Errorf("DataDir = %q, want %q", cfg.DataDir, want)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.toml")
	tests := []struct {
		name    string
		file    string
		env     map[string]string
		args    []string
		wantErr string
	}{
		{name: "explicit config file missing (flag)", args: []string{"-config", missing}, wantErr: "missing.toml"},
		{name: "explicit config file missing (env)", env: map[string]string{"WAYPOINT_CONFIG": missing}, wantErr: "missing.toml"},
		{name: "unknown key", file: "port = 1\nprot = 2\n", wantErr: "unknown keys"},
		{name: "invalid toml", file: "port = \n", wantErr: "config file"},
		{name: "file port zero", file: "port = 0\n", wantErr: "port"},
		{name: "file port wrong type", file: "port = 'abc'\n", wantErr: "config file"},
		{name: "file empty data_dir", file: "data_dir = ''\n", wantErr: "data_dir"},
		{name: "file bad log level", file: "log_level = 'loud'\n", wantErr: "log_level"},
		{name: "file cap zero", file: "active_project_cap = 0\n", wantErr: "active_project_cap"},
		{name: "file cap negative", file: "active_project_cap = -1\n", wantErr: "active_project_cap"},
		{name: "file mock latency not a duration", file: "mock_latency = 'soon'\n", wantErr: "mock_latency"},
		{name: "file mock latency negative", file: "mock_latency = '-1s'\n", wantErr: "mock_latency"},
		{name: "file snapshot recent entries negative", file: "snapshot_recent_entries = -1\n", wantErr: "snapshot_recent_entries"},
		{name: "env port too high", env: map[string]string{"WAYPOINT_PORT": "70000"}, wantErr: "WAYPOINT_PORT"},
		{name: "env port not a number", env: map[string]string{"WAYPOINT_PORT": "abc"}, wantErr: "WAYPOINT_PORT"},
		{name: "env bad log level", env: map[string]string{"WAYPOINT_LOG_LEVEL": "loud"}, wantErr: "WAYPOINT_LOG_LEVEL"},
		{name: "flag port zero", args: []string{"-port", "0"}, wantErr: "-port"},
		{name: "flag empty data dir", args: []string{"-data-dir", ""}, wantErr: "-data-dir"},
		{name: "flag bad log level", args: []string{"-log-level", "loud"}, wantErr: "-log-level"},
		{name: "empty config flag", args: []string{"-config", ""}, wantErr: "-config"},
		{name: "no home and no data dir override", env: map[string]string{"XDG_DATA_HOME": ""}, wantErr: "default data dir"},
		{name: "unknown flag", args: []string{"-nope"}, wantErr: "parse flags"},
		{name: "positional argument", args: []string{"serve"}, wantErr: "unexpected arguments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, configDir := testEnv(t, tt.env)
			if tt.file != "" {
				writeFile(t, filepath.Join(configDir, "config.toml"), tt.file)
			}
			_, err := Load(tt.args, env, "linux", io.Discard)
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadHelp(t *testing.T) {
	env, _ := testEnv(t, nil)
	var out strings.Builder
	_, err := Load([]string{"-h"}, env, "linux", &out)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("err = %v, want flag.ErrHelp", err)
	}
	if !strings.Contains(out.String(), "-data-dir") {
		t.Errorf("usage output missing flags:\n%s", out.String())
	}
}

func TestLoadWithoutHome(t *testing.T) {
	// No HOME/XDG vars: the default config file is skipped, and an explicit
	// data dir is enough to start.
	dir := t.TempDir()
	cfg, err := Load([]string{"-data-dir", dir}, fakeEnv(nil), "linux", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != dir || cfg.File != "" {
		t.Errorf("got DataDir %q File %q, want %q and no file", cfg.DataDir, cfg.File, dir)
	}
}

func TestDefaultDirs(t *testing.T) {
	tests := []struct {
		name       string
		goos       string
		env        map[string]string
		wantConfig string
		wantData   string
	}{
		{
			name:       "windows",
			goos:       "windows",
			env:        map[string]string{"APPDATA": "/roaming", "LOCALAPPDATA": "/local"},
			wantConfig: filepath.Join("/roaming", "Waypoint"),
			wantData:   filepath.Join("/local", "Waypoint"),
		},
		{
			name:       "darwin",
			goos:       "darwin",
			env:        map[string]string{"HOME": "/home/u"},
			wantConfig: filepath.Join("/home/u", "Library", "Application Support", "Waypoint"),
			wantData:   filepath.Join("/home/u", "Library", "Application Support", "Waypoint"),
		},
		{
			name:       "linux",
			goos:       "linux",
			env:        map[string]string{"HOME": "/home/u"},
			wantConfig: filepath.Join("/home/u", ".config", "waypoint"),
			wantData:   filepath.Join("/home/u", ".local", "share", "waypoint"),
		},
		{
			name:       "linux/xdg",
			goos:       "linux",
			env:        map[string]string{"HOME": "/home/u", "XDG_CONFIG_HOME": "/xc", "XDG_DATA_HOME": "/xd"},
			wantConfig: filepath.Join("/xc", "waypoint"),
			wantData:   filepath.Join("/xd", "waypoint"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := fakeEnv(tt.env)
			if got, err := defaultConfigDir(tt.goos, env); err != nil || got != tt.wantConfig {
				t.Errorf("defaultConfigDir = %q, %v; want %q", got, err, tt.wantConfig)
			}
			if got, err := defaultDataDir(tt.goos, env); err != nil || got != tt.wantData {
				t.Errorf("defaultDataDir = %q, %v; want %q", got, err, tt.wantData)
			}
		})
	}
}

func TestDefaultDirsMissingEnv(t *testing.T) {
	empty := fakeEnv(nil)
	for _, goos := range []string{"windows", "darwin", "linux"} {
		if _, err := defaultConfigDir(goos, empty); err == nil {
			t.Errorf("%s: defaultConfigDir with empty env: want error", goos)
		}
		if _, err := defaultDataDir(goos, empty); err == nil {
			t.Errorf("%s: defaultDataDir with empty env: want error", goos)
		}
	}
}

func TestEnsureDataDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	if err := EnsureDataDir(Config{DataDir: dir}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", dir)
	}
	// Idempotent.
	if err := EnsureDataDir(Config{DataDir: dir}); err != nil {
		t.Fatal(err)
	}
}

func ptr[T any](v T) *T { return &v }
