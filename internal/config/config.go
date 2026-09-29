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

	"github.com/BurntSushi/toml"
)

// DefaultPort is the port the server listens on when none is configured.
const DefaultPort = 7420

// Config is the resolved configuration. Each field comes from the highest
// precedence source that sets it: flags > WAYPOINT_* env vars > config.toml > defaults.
type Config struct {
	Port     int
	DataDir  string // absolute
	LogLevel slog.Level
	File     string // config file that was read; empty if none was found
}

// Addr is the listen address. The server always binds to loopback.
func (c Config) Addr() string {
	return "127.0.0.1:" + strconv.Itoa(c.Port)
}

// DBPath is the SQLite database file inside the data directory.
func (c Config) DBPath() string {
	return filepath.Join(c.DataDir, "waypoint.db")
}

// fileConfig mirrors config.toml. Pointers distinguish "absent" from "zero".
type fileConfig struct {
	Port     *int    `toml:"port"`
	DataDir  *string `toml:"data_dir"`
	LogLevel *string `toml:"log_level"`
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
	cfg := Config{Port: DefaultPort, LogLevel: slog.LevelInfo}

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
	} else if path, ok := nonEmpty(env, "WAYPOINT_CONFIG"); ok {
		if err := applyFile(&cfg, path); err != nil {
			return Config{}, fmt.Errorf("config file %s: %w", path, err)
		}
		cfg.File = path
	} else if dir, err := defaultConfigDir(goos, env); err == nil {
		path := filepath.Join(dir, "config.toml")
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
	return nil
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
