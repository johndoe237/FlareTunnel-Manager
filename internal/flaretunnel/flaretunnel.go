// Package flaretunnel is the orchestration layer for the FlareTunnel binary.
//
// It builds the exact commands and configuration files that FlareTunnel
// expects, and executes them. It never re-implements FlareTunnel logic: it
// only prepares data and delegates the work to the FlareTunnel binary.
package flaretunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

const (
	ConfigFile    = "flaretunnel.json"
	EndpointsFile = "flaretunnel_endpoints.json"
)

type Account struct {
	Name      string `json:"name"`
	APIToken  string `json:"api_token"`
	AccountID string `json:"account_id"`
	ZoneID    string `json:"zone_id,omitempty"`
}

type Config struct {
	Accounts []Account `json:"accounts"`
}

type Endpoint struct {
	Name              string `json:"name"`
	URL               string `json:"url"`
	CreatedAt         string `json:"created_at"`
	ID                string `json:"id"`
	AccountID         string `json:"account_id"`
	ConfigAccountName string `json:"config_account_name,omitempty"`
}

type Runner struct{ Binary string }

func New(binary string) *Runner {
	if binary == "" {
		binary = "flaretunnel"
	}
	return &Runner{Binary: binary}
}

func WriteConfig(dir, name, apiToken, accountID, zoneID string) error {
	cfg := Config{Accounts: []Account{{Name: name, APIToken: apiToken, AccountID: accountID, ZoneID: zoneID}}}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ConfigFile), data, 0o600)
}

func ReadEndpoints(path string) ([]Endpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var eps []Endpoint
	if err := json.Unmarshal(data, &eps); err != nil {
		return nil, err
	}
	return eps, nil
}

func WriteEndpoints(path string, eps []Endpoint) error {
	data, err := json.MarshalIndent(eps, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (r *Runner) Create(ctx context.Context, dir, account string, count int) error {
	return r.run(ctx, dir, []string{"create", "--count", strconv.Itoa(count), "--account", account})
}

// Cleanup removes at most count Workers. Automated callers must always provide
// the bound and --yes; the historical unbounded cleanup is intentionally not
// exposed through this interface.
func (r *Runner) Cleanup(ctx context.Context, dir, account string, count int) error {
	args := []string{"cleanup", "--account", account, "--count", strconv.Itoa(count), "--yes"}
	return r.run(ctx, dir, args)
}

func (r *Runner) List(ctx context.Context, dir string) error {
	return r.run(ctx, dir, []string{"list"})
}

func (r *Runner) TunnelArgs(port int, mode, blacklistFile string) []string {
	return []string{"tunnel", "--port", strconv.Itoa(port), "--mode", mode, "--blacklist", blacklistFile}
}

func (r *Runner) Launch(ctx context.Context, dir string, args []string, env map[string]string) error {
	bin, err := exec.LookPath(r.Binary)
	if err != nil {
		if _, statErr := os.Stat(r.Binary); statErr != nil {
			return fmt.Errorf("flaretunnel binary not found: %v", err)
		}
		bin = r.Binary
	}
	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("cannot chdir to runtime dir: %w", err)
	}
	processEnv := os.Environ()
	for key, value := range env {
		processEnv = append(processEnv, key+"="+value)
	}
	return syscall.Exec(bin, append([]string{bin}, args...), processEnv)
}

func (r *Runner) run(ctx context.Context, dir string, args []string) error {
	cmd := exec.CommandContext(ctx, r.Binary, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("flaretunnel %v failed: %v: %s", args, err, sanitizeOutput(string(out)))
	}
	return nil
}

func sanitizeOutput(s string) string {
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || value == "" || !isSensitiveEnvKey(key) {
			continue
		}
		s = strings.ReplaceAll(s, value, "[REDACTED]")
	}
	sensitiveField := regexp.MustCompile(`(?i)(api[_-]?token|token|secret|password|credential)(["' ]*[:=]["' ]*)[^,\s}"']+`)
	s = sensitiveField.ReplaceAllString(s, `$1$2[REDACTED]`)
	if len(s) > 2000 {
		s = s[:2000] + "..."
	}
	return s
}

func isSensitiveEnvKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "password") || strings.Contains(key, "credential") || strings.Contains(key, "api_key") || strings.Contains(key, "apikey")
}

// SanitizeOutput is exported for the logging layer and tests.
func SanitizeOutput(s string) string { return sanitizeOutput(s) }
