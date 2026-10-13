package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// providerKeys are the config.toml keys SaveProvider may set, in the order
// missing ones are added.
var providerKeys = [...]string{"provider", "model", "endpoint", "api_key"}

// topLevelKey matches a bare key at the start of a "key = value" line.
var topLevelKey = regexp.MustCompile(`^\s*([A-Za-z0-9_-]+)\s*=`)

// SaveProvider sets the provider keys in values ("provider", "model",
// "endpoint", or "api_key") in the config file at path, creating it if needed. An empty value removes
// the key. Only those keys' lines change, so the user's comments and other
// settings stay as they were; a key not in the file yet is added at the end
// of the top-level keys. The file is replaced atomically and readable only by
// the owner, since it may hold an API key.
func SaveProvider(path string, values map[string]string) error {
	for k := range values {
		if !slices.Contains(providerKeys[:], k) {
			return fmt.Errorf("save provider: %q is not a provider key", k)
		}
	}
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("save provider: %w", err)
	}
	updated, err := setKeys(string(old), values)
	if err != nil {
		return fmt.Errorf("save provider: %w", err)
	}
	if err := writeAtomic(path, []byte(updated)); err != nil {
		return fmt.Errorf("save provider: %w", err)
	}
	return nil
}

// setKeys returns doc with the top-level keys in values set, removed (empty
// values), or added, and checks that the result decodes to those values.
func setKeys(doc string, values map[string]string) (string, error) {
	nl := "\n"
	if strings.Contains(doc, "\r\n") {
		nl = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(doc, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	// Top-level keys come before the first table header. A continuation line
	// of a multi-line array can look like one; the decode check below then
	// refuses the result rather than writing a broken file.
	end := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(strings.TrimSpace(l), "[") })
	if end < 0 {
		end = len(lines)
	}
	done := map[string]bool{}
	var out []string
	for i, l := range lines {
		if i < end {
			if m := topLevelKey.FindStringSubmatch(l); m != nil {
				if v, ok := values[m[1]]; ok {
					done[m[1]] = true
					if v != "" {
						out = append(out, keyValue(m[1], v))
					}
					continue
				}
			}
		}
		if i == end {
			out = appendMissing(out, values, done)
		}
		out = append(out, l)
	}
	if end == len(lines) {
		out = appendMissing(out, values, done)
	}
	result := strings.Join(out, nl)
	if result != "" {
		result += nl
	}
	var got map[string]any
	if _, err := toml.Decode(result, &got); err != nil {
		return "", fmt.Errorf("updated config doesn't parse: %w", err)
	}
	for k, v := range values {
		s, _ := got[k].(string)
		if s != v {
			return "", fmt.Errorf("updated config has the wrong %s", k)
		}
	}
	return result, nil
}

// appendMissing adds the non-empty values not yet written, in providerKeys order.
func appendMissing(out []string, values map[string]string, done map[string]bool) []string {
	for _, k := range providerKeys {
		v, ok := values[k]
		if !ok || v == "" || done[k] {
			continue
		}
		out = append(out, keyValue(k, v))
		done[k] = true
	}
	return out
}

// keyValue is `key = "value"`, the value quoted as a TOML string.
func keyValue(key, value string) string {
	var b bytes.Buffer
	// Encoding a string map into a buffer can't fail; setKeys decodes the
	// result to check it anyway.
	_ = toml.NewEncoder(&b).Encode(map[string]string{key: value})
	return strings.TrimRight(b.String(), "\n")
}

// writeAtomic writes data to a temp file beside path, owner-only, and
// renames it over path.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".config-*.toml")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // after a successful rename there's nothing to remove
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
