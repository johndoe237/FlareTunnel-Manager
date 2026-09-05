// Package runtime manages the temporary filesystem layout used by the manager.
//
// Each account gets its own runtime directory so that configuration files
// never mix between accounts. Directories are removed once the account has
// been fully processed (or on failure), and the whole tree is cleaned at
// process exit so that no temporary credential is left on disk.
package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manager owns the base runtime directory.
type Manager struct {
	Base string
}

// New creates (or reuses) the base runtime directory. An empty base falls
// back to a directory under the system temporary directory.
func New(base string) (*Manager, error) {
	if base == "" {
		base = filepath.Join(os.TempDir(), "flaretunnel-manager")
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return nil, fmt.Errorf("cannot create runtime directory %s: %w", base, err)
	}
	return &Manager{Base: base}, nil
}

// NewAccountDir creates and returns a dedicated directory for one account.
// The directory name is sanitized so that account names cannot collide or
// escape the base directory.
func (m *Manager) NewAccountDir(accountName string) (string, error) {
	dir := filepath.Join(m.Base, sanitize(accountName))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("cannot create account runtime directory: %w", err)
	}
	return dir, nil
}

// UseDir returns the single runtime directory used by MODE=use. It must stay
// available after the exec of FlareTunnel (endpoints file and blacklist).
func (m *Manager) UseDir() (string, error) {
	dir := filepath.Join(m.Base, "use")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("cannot create use runtime directory: %w", err)
	}
	return dir, nil
}

// RemoveDir removes an account runtime directory.
func (m *Manager) RemoveDir(dir string) error {
	return os.RemoveAll(dir)
}

// CleanupAll removes the whole runtime tree. It is called at process exit.
func (m *Manager) CleanupAll() {
	_ = os.RemoveAll(m.Base)
}

// sanitize turns an account name into a safe directory name.
func sanitize(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	s := b.String()
	if s == "" || s == "." || s == ".." {
		return "account"
	}
	return s
}
