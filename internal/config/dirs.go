package config

import (
	"errors"
	"path/filepath"
)

// LookupEnv has the signature of os.LookupEnv. It is passed in so tests can
// supply a fake environment.
type LookupEnv func(key string) (string, bool)

// defaultConfigDir returns the OS-appropriate directory for config.toml:
//
//	windows: %APPDATA%\Waypoint
//	darwin:  ~/Library/Application Support/Waypoint
//	other:   $XDG_CONFIG_HOME/waypoint, or ~/.config/waypoint
func defaultConfigDir(goos string, env LookupEnv) (string, error) {
	switch goos {
	case "windows":
		return fromEnv(env, "APPDATA", "Waypoint")
	case "darwin":
		return fromHome(env, "Library", "Application Support", "Waypoint")
	default:
		if dir, ok := nonEmpty(env, "XDG_CONFIG_HOME"); ok {
			return filepath.Join(dir, "waypoint"), nil
		}
		return fromHome(env, ".config", "waypoint")
	}
}

// defaultDataDir returns the OS-appropriate directory for the database:
//
//	windows: %LOCALAPPDATA%\Waypoint
//	darwin:  ~/Library/Application Support/Waypoint
//	other:   $XDG_DATA_HOME/waypoint, or ~/.local/share/waypoint
func defaultDataDir(goos string, env LookupEnv) (string, error) {
	switch goos {
	case "windows":
		return fromEnv(env, "LOCALAPPDATA", "Waypoint")
	case "darwin":
		return fromHome(env, "Library", "Application Support", "Waypoint")
	default:
		if dir, ok := nonEmpty(env, "XDG_DATA_HOME"); ok {
			return filepath.Join(dir, "waypoint"), nil
		}
		return fromHome(env, ".local", "share", "waypoint")
	}
}

func fromEnv(env LookupEnv, key string, elem ...string) (string, error) {
	dir, ok := nonEmpty(env, key)
	if !ok {
		return "", errors.New("%" + key + "% is not set")
	}
	return filepath.Join(append([]string{dir}, elem...)...), nil
}

func fromHome(env LookupEnv, elem ...string) (string, error) {
	home, ok := nonEmpty(env, "HOME")
	if !ok {
		return "", errors.New("$HOME is not set")
	}
	return filepath.Join(append([]string{home}, elem...)...), nil
}

func nonEmpty(env LookupEnv, key string) (string, bool) {
	v, ok := env(key)
	return v, ok && v != ""
}
