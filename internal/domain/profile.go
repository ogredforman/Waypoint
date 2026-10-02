package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// ProfileNameMax is the longest a profile's display name may be, in characters.
const ProfileNameMax = 60

// profileIDMax is the longest a profile ID may be, in bytes.
const profileIDMax = 40

// Profile is one of the user's profiles: a separate database with a display
// name (ADR 0005). ID names the profile in API paths and is its file name
// without ".db"; it is made from the name at creation and never changes.
type Profile struct {
	ID   string
	Name string
}

// CleanProfileName trims name and checks it: not blank and at most
// ProfileNameMax characters. Uniqueness across profiles is checked by the
// caller, which knows the other profiles.
func CleanProfileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	var c checker
	switch n := utf8.RuneCountInString(name); {
	case n == 0:
		c.add("name", "is required")
	case n > ProfileNameMax:
		c.add("name", "must be at most %d characters, got %d", ProfileNameMax, n)
	}
	return name, c.err("profile")
}

// ProfileSlug turns a display name into a profile ID: lowercase ASCII letters
// and digits, with runs of anything else as one hyphen, trimmed to
// profileIDMax bytes. A name with no usable characters gives "profile".
func ProfileSlug(name string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			hyphen = false
			b.WriteRune(r)
			continue
		}
		hyphen = true
	}
	s := b.String()
	if len(s) > profileIDMax {
		s = strings.TrimRight(s[:profileIDMax], "-")
	}
	if s == "" {
		return "profile"
	}
	if reservedFileName(s) {
		return s + "-profile"
	}
	return s
}

// reservedFileName reports whether Windows reserves name for a device, with
// any extension (so "con.db" can't be a file either).
func reservedFileName(name string) bool {
	switch name {
	case "con", "prn", "aux", "nul":
		return true
	}
	if len(name) == 4 && (strings.HasPrefix(name, "com") || strings.HasPrefix(name, "lpt")) {
		return name[3] >= '1' && name[3] <= '9'
	}
	return false
}

// ValidProfileID reports whether id has the shape ProfileSlug produces, with
// an optional numeric suffix such as "-2". Anything else can't name a profile
// file, so it is never looked up on disk. Windows device names (con,
// nul, com1, …) are never IDs.
func ValidProfileID(id string) bool {
	if id == "" || len(id) > profileIDMax+4 || id[0] == '-' || id[len(id)-1] == '-' || reservedFileName(id) {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return !strings.Contains(id, "--")
}

// ErrLastProfile is returned when deleting the only profile left: the server
// always has at least one.
var ErrLastProfile = errors.New("the last profile can't be deleted")
