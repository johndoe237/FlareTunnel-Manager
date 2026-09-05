package flaretunnel

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWriteConfigSingleAccount(t *testing.T) {
	dir := t.TempDir()
	if err := WriteConfig(dir, "acc-a", "tok-a", "id-a", "zone-a"); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "flaretunnel.json"))
	if err != nil {
		t.Fatalf("read flaretunnel.json: %v", err)
	}
	if !strings.Contains(string(data), `"name": "acc-a"`) ||
		!strings.Contains(string(data), `"api_token": "tok-a"`) ||
		!strings.Contains(string(data), `"account_id": "id-a"`) {
		t.Errorf("flaretunnel.json content missing expected fields:\n%s", data)
	}
	info, err := os.Stat(filepath.Join(dir, "flaretunnel.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("flaretunnel.json perms = %v, want 0600", info.Mode().Perm())
	}
}

func TestEndpointsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flaretunnel_endpoints.json")
	eps := []Endpoint{
		{Name: "flaretunnel-1-x", URL: "https://flaretunnel-1-x.sub.workers.dev", ID: "flaretunnel-1-x", AccountID: "id-a", ConfigAccountName: "acc-a"},
	}
	if err := WriteEndpoints(path, eps); err != nil {
		t.Fatalf("WriteEndpoints() error = %v", err)
	}
	got, err := ReadEndpoints(path)
	if err != nil {
		t.Fatalf("ReadEndpoints() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "flaretunnel-1-x" || got[0].ConfigAccountName != "acc-a" {
		t.Errorf("unexpected endpoints: %+v", got)
	}
}

func TestReadEndpointsMissing(t *testing.T) {
	if _, err := ReadEndpoints(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected error for missing endpoints file")
	}
}

func TestTunnelArgsTransmitPortAndMode(t *testing.T) {
	r := New("flaretunnel")
	args := r.TunnelArgs(9090, "random", "/opt/flaretunnel/blacklist-minimal.txt")
	want := []string{"tunnel", "--port", "9090", "--mode", "random", "--blacklist", "/opt/flaretunnel/blacklist-minimal.txt"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("TunnelArgs() = %v, want %v", args, want)
	}
}

func TestCleanupArgsIncludeExactBoundAndYes(t *testing.T) {
	// Automated cleanup must be bounded and non-interactive.
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" > args.txt\n"
	bin := filepath.Join(dir, "fake-flaretunnel")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := New(bin)
	if err := r.Cleanup(context.Background(), dir, "acc-a", 7); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "args.txt"))
	s := strings.TrimSpace(string(got))
	if s != "cleanup --account acc-a --count 7 --yes" {
		t.Errorf("cleanup args = %q, want exact bounded cleanup arguments", s)
	}
}

func TestCreateArgs(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" > args.txt\n"
	bin := filepath.Join(dir, "fake-flaretunnel")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := New(bin)
	if err := r.Create(context.Background(), dir, "acc-a", 20); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "args.txt"))
	s := strings.TrimSpace(string(got))
	if s != "create --count 20 --account acc-a" {
		t.Errorf("create args = %q", s)
	}
}

func TestRunnerErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "failing")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := New(bin)
	if err := r.List(context.Background(), dir); err == nil {
		t.Fatal("expected error from failing binary")
	}
}

func TestSanitizeOutputRedactsEnvironmentSecrets(t *testing.T) {
	t.Setenv("CF_API_TOKEN", "do-not-log-this-token")
	got := SanitizeOutput(`api_token: do-not-log-this-token`)
	if strings.Contains(got, "do-not-log-this-token") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("sanitized output = %q, secret was not redacted", got)
	}
}
