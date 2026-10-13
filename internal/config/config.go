package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/ogredforman/waypoint/internal/domain"
)

// DefaultPort is the port the server listens on when none is configured.
const DefaultPort = 7420

// DefaultSnapshotRecentEntries is how many recent entries go into an
// extraction snapshot when none is configured.
const DefaultSnapshotRecentEntries = 5

// DefaultStalledAfterDays is how many days an in-progress task can go
// unchanged before extraction sees it as stalled, when none is configured.
const DefaultStalledAfterDays = 14

// Config is the resolved configuration. Each field comes from the highest
// precedence source that sets it: flags > WAYPOINT_* env vars > config.toml > defaults.
type Config struct {
	Port     int
	DataDir  string // absolute
	LogLevel slog.Level
	File     string // config file that was read; empty if none was found
	// WritePath is the config file settings changed in the app are saved to:
	// File, or where the default one would be if it doesn't exist yet. Empty
	// if no location could be determined.
	WritePath string

	// ActiveProjectCap and QuestionsPerEntry only seed the settings of a
	// database that has none yet (SettingsDefaults); after that the stored
	// settings, changed through PATCH /api/settings, are in effect. They are
	// set in config.toml only (no env var or flag).
	ActiveProjectCap  int
	QuestionsPerEntry int

	// MockLatency is an artificial delay the mock extractor adds to each run,
	// to exercise the async UI. It is set in config.toml only.
	MockLatency time.Duration

	// SnapshotRecentEntries is how many earlier entries, as observation
	// summaries, the extractor sees for context. 0 means none. It is set in
	// config.toml only.
	SnapshotRecentEntries int

	// StalledAfterDays is how many days an in-progress task, steps included,
	// can go unchanged before the extractor is told it has stalled, which
	// makes it a candidate for a split_task suggestion. 0 marks none. It is
	// set in config.toml only.
	StalledAfterDays int

	// Provider, Model, Endpoint, and APIKey choose the extractor (see
	// extract/provider). An empty Provider means the mock. They are set in
	// config.toml or WAYPOINT_* env vars, never flags, so a key stays out of
	// shell history. APIKey must never be logged.
	Provider string
	Model    string
	Endpoint string
	APIKey   string
	// ProviderFromEnv names the provider keys ("provider", "model",
	// "endpoint", "api_key") that WAYPOINT_* env vars set, in that order.
	ProviderFromEnv []string

	// Timeout bounds one model request; 0 means the provider's default. It
	// is set in config.toml or WAYPOINT_TIMEOUT.
	Timeout time.Duration
}

// Addr is the listen address. The server always binds to loopback.
func (c Config) Addr() string {
	return "127.0.0.1:" + strconv.Itoa(c.Port)
}

// SettingsDefaults are the settings a database without any is seeded with:
// the config's caps, and the built-in defaults for everything config.toml
// doesn't set (the fonts).
func (c Config) SettingsDefaults() domain.Settings {
	s := domain.DefaultSettings()
	s.ActiveProjectCap = c.ActiveProjectCap
	s.QuestionsPerEntry = c.QuestionsPerEntry
	return s
}

// StalledAfter is StalledAfterDays as a duration.
func (c Config) StalledAfter() time.Duration {
	return time.Duration(c.StalledAfterDays) * 24 * time.Hour
}

// ProfilesDir holds one SQLite database per profile (ADR 0005).
func (c Config) ProfilesDir() string {
	return filepath.Join(c.DataDir, "profiles")
}

// LegacyDBPath is the database from before profiles. If it exists at startup
// it is moved into ProfilesDir as the profile "Personal".
func (c Config) LegacyDBPath() string {
	return filepath.Join(c.DataDir, "waypoint.db")
}

// fileConfig mirrors config.toml. Pointers distinguish "absent" from "zero".
type fileConfig struct {
	Port     *int    `toml:"port"`
	DataDir  *string `toml:"data_dir"`
	LogLevel *string `toml:"log_level"`

	ActiveProjectCap  *int    `toml:"active_project_cap"`
	QuestionsPerEntry *int    `toml:"questions_per_entry"`
	MockLatency       *string `toml:"mock_latency"`

	SnapshotRecentEntries *int `toml:"snapshot_recent_entries"`
	StalledAfterDays      *int `toml:"stalled_after_days"`

	Provider *string `toml:"provider"`
	Model    *string `toml:"model"`
	Endpoint *string `toml:"endpoint"`
	APIKey   *string `toml:"api_key"`
	Timeout  *string `toml:"timeout"`
}

// Load resolves the configuration from args (without the program name), the
// environment, and the config file. goos selects the OS-specific default
// directories. Usage and flag errors are written to stderr. For -h it returns
// an error wrapping flag.ErrHelp.
func Load(args []string, env LookupEnv, goos string, stderr io.Writer) (Config, error) {
	fset := flag.NewFlagSet("waypoint", flag.ContinueOnError)
	fset.SetOutput(stderr)
	flagConfig := fset.String("config", "", "path to config.toml (env WAYPOINT_CONFIG)")
	flagPort := fset.String("port", "", fmt.Sprintf("port to listen on (env WAYPOINT_PORT, default %d)", DefaultPort))
	flagDataDir := fset.String("data-dir", "", "directory for the database (env WAYPOINT_DATA_DIR)")
	flagLogLevel := fset.String("log-level", "", "debug, info, warn, or error (env WAYPOINT_LOG_LEVEL, default info)")
	if err := fset.Parse(args); err != nil {
		return Config{}, fmt.Errorf("parse flags: %w", err)
	}
	if fset.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected arguments: %v", fset.Args())
	}
	set := map[string]bool{}
	fset.Visit(func(f *flag.Flag) { set[f.Name] = true })

	// DataDir stays empty until a source sets it; the OS default is filled in
	// last, so a missing $HOME only matters when nothing else names the folder.
	cfg := Config{Port: DefaultPort, LogLevel: slog.LevelInfo, ActiveProjectCap: domain.DefaultActiveProjectCap, QuestionsPerEntry: domain.MaxQuestionsPerEntry, SnapshotRecentEntries: DefaultSnapshotRecentEntries, StalledAfterDays: DefaultStalledAfterDays}

	// Config file: an explicitly named file must exist. The default one is
	// optional, and skipped if the OS config dir can't be determined.
	if set["config"] {
		if *flagConfig == "" {
			return Config{}, errors.New("flag -config: must not be empty")
		}
		if err := applyFile(&cfg, *flagConfig); err != nil {
			return Config{}, fmt.Errorf("config file %s: %w", *flagConfig, err)
		}
		cfg.File = *flagConfig
		cfg.WritePath = cfg.File
	} else if path, ok := nonEmpty(env, "WAYPOINT_CONFIG"); ok {
		if err := applyFile(&cfg, path); err != nil {
			return Config{}, fmt.Errorf("config file %s: %w", path, err)
		}
		cfg.File = path
		cfg.WritePath = cfg.File
	} else if dir, err := defaultConfigDir(goos, env); err == nil {
		path := filepath.Join(dir, "config.toml")
		cfg.WritePath = path
		err := applyFile(&cfg, path)
		switch {
		case err == nil:
			cfg.File = path
		case !errors.Is(err, fs.ErrNotExist):
			return Config{}, fmt.Errorf("config file %s: %w", path, err)
		}
	}

	// Env, then flags, each overriding what came before.
	var fromEnv overrides
	if v, ok := nonEmpty(env, "WAYPOINT_PORT"); ok {
		fromEnv.port = &v
	}
	if v, ok := nonEmpty(env, "WAYPOINT_DATA_DIR"); ok {
		fromEnv.dataDir = &v
	}
	if v, ok := nonEmpty(env, "WAYPOINT_LOG_LEVEL"); ok {
		fromEnv.logLevel = &v
	}
	if err := fromEnv.apply(&cfg, "WAYPOINT_PORT", "WAYPOINT_DATA_DIR", "WAYPOINT_LOG_LEVEL"); err != nil {
		return Config{}, fmt.Errorf("env %w", err)
	}

	for _, p := range []struct {
		env, key string
		field    *string
	}{
		{"WAYPOINT_PROVIDER", "provider", &cfg.Provider},
		{"WAYPOINT_MODEL", "model", &cfg.Model},
		{"WAYPOINT_ENDPOINT", "endpoint", &cfg.Endpoint},
		{"WAYPOINT_API_KEY", "api_key", &cfg.APIKey},
	} {
		if v, ok := nonEmpty(env, p.env); ok {
			*p.field = v
			cfg.ProviderFromEnv = append(cfg.ProviderFromEnv, p.key)
		}
	}
	if v, ok := nonEmpty(env, "WAYPOINT_TIMEOUT"); ok {
		d, err := parseTimeout(v)
		if err != nil {
			return Config{}, fmt.Errorf("env WAYPOINT_TIMEOUT: %w", err)
		}
		cfg.Timeout = d
	}

	var fromFlags overrides
	if set["port"] {
		fromFlags.port = flagPort
	}
	if set["data-dir"] {
		fromFlags.dataDir = flagDataDir
	}
	if set["log-level"] {
		fromFlags.logLevel = flagLogLevel
	}
	if err := fromFlags.apply(&cfg, "-port", "-data-dir", "-log-level"); err != nil {
		return Config{}, fmt.Errorf("flag %w", err)
	}

	if cfg.DataDir == "" {
		dir, err := defaultDataDir(goos, env)
		if err != nil {
			return Config{}, fmt.Errorf("default data dir: %w (set -data-dir or WAYPOINT_DATA_DIR)", err)
		}
		cfg.DataDir = dir
	}
	return cfg, nil
}

// overrides holds string values from env vars or flags. nil means "not set".
type overrides struct {
	port, dataDir, logLevel *string
}

// apply overlays the set values onto cfg. The names are used in error messages.
func (o overrides) apply(cfg *Config, portName, dataDirName, logLevelName string) error {
	var err error
	if o.port != nil {
		if cfg.Port, err = parsePort(*o.port); err != nil {
			return fmt.Errorf("%s: %w", portName, err)
		}
	}
	if o.dataDir != nil {
		if *o.dataDir == "" {
			return fmt.Errorf("%s: must not be empty", dataDirName)
		}
		if cfg.DataDir, err = filepath.Abs(*o.dataDir); err != nil {
			return fmt.Errorf("%s: %w", dataDirName, err)
		}
	}
	if o.logLevel != nil {
		if cfg.LogLevel, err = parseLevel(*o.logLevel); err != nil {
			return fmt.Errorf("%s: %w", logLevelName, err)
		}
	}
	return nil
}

// applyFile overlays the values present in the TOML file at path onto cfg.
// A relative data_dir is resolved against the file's directory.
func applyFile(cfg *Config, path string) error {
	var fc fileConfig
	md, err := toml.DecodeFile(path, &fc)
	if err != nil {
		return err
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return fmt.Errorf("unknown keys: %v", undecoded)
	}
	if fc.Port != nil {
		if err := validPort(*fc.Port); err != nil {
			return fmt.Errorf("port: %w", err)
		}
		cfg.Port = *fc.Port
	}
	if fc.DataDir != nil {
		if *fc.DataDir == "" {
			return errors.New("data_dir: must not be empty")
		}
		dir := *fc.DataDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(filepath.Dir(path), dir)
		}
		if cfg.DataDir, err = filepath.Abs(dir); err != nil {
			return fmt.Errorf("data_dir: %w", err)
		}
	}
	if fc.LogLevel != nil {
		if cfg.LogLevel, err = parseLevel(*fc.LogLevel); err != nil {
			return fmt.Errorf("log_level: %w", err)
		}
	}
	if fc.ActiveProjectCap != nil {
		if *fc.ActiveProjectCap < 1 {
			return fmt.Errorf("active_project_cap: must be at least 1, got %d", *fc.ActiveProjectCap)
		}
		cfg.ActiveProjectCap = *fc.ActiveProjectCap
	}
	if fc.QuestionsPerEntry != nil {
		if q := *fc.QuestionsPerEntry; q < 0 || q > domain.MaxQuestionsPerEntry {
			return fmt.Errorf("questions_per_entry: must be from 0 to %d, got %d", domain.MaxQuestionsPerEntry, q)
		}
		cfg.QuestionsPerEntry = *fc.QuestionsPerEntry
	}
	if fc.MockLatency != nil {
		d, err := time.ParseDuration(*fc.MockLatency)
		if err != nil {
			return fmt.Errorf("mock_latency: %q is not a duration such as \"1500ms\"", *fc.MockLatency)
		}
		if d < 0 {
			return fmt.Errorf("mock_latency: must not be negative, got %s", d)
		}
		cfg.MockLatency = d
	}
	if fc.SnapshotRecentEntries != nil {
		if *fc.SnapshotRecentEntries < 0 {
			return fmt.Errorf("snapshot_recent_entries: must not be negative, got %d", *fc.SnapshotRecentEntries)
		}
		cfg.SnapshotRecentEntries = *fc.SnapshotRecentEntries
	}
	if fc.StalledAfterDays != nil {
		if *fc.StalledAfterDays < 0 {
			return fmt.Errorf("stalled_after_days: must not be negative, got %d", *fc.StalledAfterDays)
		}
		cfg.StalledAfterDays = *fc.StalledAfterDays
	}
	for field, v := range map[*string]*string{
		&cfg.Provider: fc.Provider,
		&cfg.Model:    fc.Model,
		&cfg.Endpoint: fc.Endpoint,
		&cfg.APIKey:   fc.APIKey,
	} {
		if v != nil {
			*field = *v
		}
	}
	if fc.Timeout != nil {
		if cfg.Timeout, err = parseTimeout(*fc.Timeout); err != nil {
			return fmt.Errorf("timeout: %w", err)
		}
	}
	return nil
}

func parseTimeout(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a duration such as \"5m\"", s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("must be positive, got %s", d)
	}
	return d, nil
}

func parsePort(s string) (int, error) {
	p, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", s)
	}
	return p, validPort(p)
}

func validPort(p int) error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("%d is outside 1-65535", p)
	}
	return nil
}

func parseLevel(s string) (slog.Level, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("%q is not one of debug, info, warn, error", s)
	}
	return l, nil
}

// EnsureDataDir creates the data directory, readable only by the owner.
func EnsureDataDir(cfg Config) error {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	return nil
}
