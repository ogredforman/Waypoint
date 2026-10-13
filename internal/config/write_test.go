package config

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveProvider(t *testing.T) {
	tests := []struct {
		name   string
		file   string // "" for no file
		values map[string]string
		want   string
	}{
		{
			name:   "creates the file",
			values: map[string]string{"provider": "anthropic", "api_key": "sk-1", "model": ""},
			want:   "provider = \"anthropic\"\napi_key = \"sk-1\"\n",
		},
		{
			name:   "replaces in place and keeps comments",
			file:   "# my settings\nport = 7421\nprovider = 'mock'   # for now\n\n# the key\napi_key = \"old\"\n",
			values: map[string]string{"provider": "openai-compatible", "api_key": "new", "endpoint": "http://localhost:11434/v1", "model": "qwen3:8b"},
			want:   "# my settings\nport = 7421\nprovider = \"openai-compatible\"\n\n# the key\napi_key = \"new\"\nmodel = \"qwen3:8b\"\nendpoint = \"http://localhost:11434/v1\"\n",
		},
		{
			name:   "empty removes the key",
			file:   "provider = \"anthropic\"\napi_key = \"sk-1\"\nport = 7421\n",
			values: map[string]string{"provider": "mock", "api_key": ""},
			want:   "provider = \"mock\"\nport = 7421\n",
		},
		{
			name:   "keys not given are left alone",
			file:   "api_key = \"from-file\"\n",
			values: map[string]string{"provider": "anthropic"},
			want:   "api_key = \"from-file\"\nprovider = \"anthropic\"\n",
		},
		{
			name:   "adds before the first table",
			file:   "port = 7421\n\n[later]\nprovider = \"not top level\"\n",
			values: map[string]string{"provider": "mock"},
			want:   "port = 7421\n\nprovider = \"mock\"\n[later]\nprovider = \"not top level\"\n",
		},
		{
			name:   "quotes and backslashes are escaped",
			values: map[string]string{"api_key": `a"b\c`},
			want:   `api_key = "a\"b\\c"` + "\n",
		},
		{
			name:   "CRLF stays CRLF",
			file:   "port = 7421\r\nmodel = \"x\"\r\n",
			values: map[string]string{"model": "y"},
			want:   "port = 7421\r\nmodel = \"y\"\r\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "waypoint", "config.toml")
			if tt.file != "" {
				writeFile(t, path, tt.file)
			}
			if err := SaveProvider(path, tt.values); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("file =\n%s\nwant\n%s", got, tt.want)
			}
			if runtime.GOOS != "windows" {
				if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
					t.Errorf("mode = %v, %v; want 0600", fi.Mode().Perm(), err)
				}
			}
			entries, _ := os.ReadDir(filepath.Dir(path))
			if len(entries) != 1 {
				t.Errorf("dir has %d files, want only config.toml", len(entries))
			}
		})
	}
}

func TestSaveProviderLoadsBack(t *testing.T) {
	env, configDir := testEnv(t, nil)
	path := filepath.Join(configDir, "config.toml")
	writeFile(t, path, "port = 7421\n")
	if err := SaveProvider(path, map[string]string{"provider": "openai-compatible", "model": "qwen3:8b", "endpoint": "http://localhost:11434/v1", "api_key": "k"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(nil, env, "linux", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got := [5]any{cfg.Port, cfg.Provider, cfg.Model, cfg.Endpoint, cfg.APIKey}; got != [5]any{7421, "openai-compatible", "qwen3:8b", "http://localhost:11434/v1", "k"} {
		t.Errorf("loaded %v", got)
	}
}

func TestSaveProviderRefuses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := SaveProvider(path, map[string]string{"port": "1"}); err == nil {
		t.Error("saved a key that isn't a provider key")
	}
	writeFile(t, path, "port = \n")
	if err := SaveProvider(path, map[string]string{"model": "x"}); err == nil {
		t.Error("saved over a file that doesn't parse")
	}
	if got, _ := os.ReadFile(path); string(got) != "port = \n" {
		t.Errorf("file changed to %q", got)
	}
}
