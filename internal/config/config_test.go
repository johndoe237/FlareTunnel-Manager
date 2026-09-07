package config

import (
	"os"
	"testing"
)

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	if kv["MODE"] == "use" {
		if _, ok := kv["AUTH_PROXY"]; !ok {
			kv["AUTH_PROXY"] = `{"username":"test-user","password":"test-password"}`
		}
	}
	for k, v := range kv {
		if err := os.Setenv(k, v); err != nil {
			t.Fatalf("setenv %s: %v", k, err)
		}
	}
}

func clearEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("unsetenv %s: %v", k, err)
		}
	}
}

func TestLoadModeSelection(t *testing.T) {
	cases := []struct {
		name string
		mode string
		want Mode
	}{
		{"create", "create", ModeCreate},
		{"use", "use", ModeUse},
		{"delete", "delete", ModeDelete},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, map[string]string{"MODE": tc.mode})
			defer clearEnv(t, "MODE")
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Mode != tc.want {
				t.Errorf("Mode = %q, want %q", cfg.Mode, tc.want)
			}
		})
	}
}

func TestLoadInvalidMode(t *testing.T) {
	setEnv(t, map[string]string{"MODE": "bogus"})
	defer clearEnv(t, "MODE")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid MODE")
	}
}

func TestLoadEmptyMode(t *testing.T) {
	clearEnv(t, "MODE")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for empty MODE")
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, map[string]string{"MODE": "create"})
	defer clearEnv(t, "MODE", "PORT", "FLARETUNNEL_MODE", "FLARETUNNEL_BLACKLIST")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.RotationMode != "random" {
		t.Errorf("RotationMode = %q, want random", cfg.RotationMode)
	}
	if cfg.BlacklistLevel != "minimal" {
		t.Errorf("BlacklistLevel = %q, want minimal", cfg.BlacklistLevel)
	}
	if cfg.BlacklistFile != "blacklist-minimal.txt" {
		t.Errorf("BlacklistFile = %q, want blacklist-minimal.txt", cfg.BlacklistFile)
	}
}

func TestLoadPort(t *testing.T) {
	setEnv(t, map[string]string{"MODE": "use", "PORT": "9090"})
	defer clearEnv(t, "MODE", "PORT")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
}

func TestLoadPortInvalidFallsBackToDefault(t *testing.T) {
	for _, bad := range []string{"abc", "-5", "0", "70000"} {
		t.Run(bad, func(t *testing.T) {
			setEnv(t, map[string]string{"MODE": "use", "PORT": bad})
			defer clearEnv(t, "MODE", "PORT")
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Port != 8080 {
				t.Errorf("Port = %d, want default 8080 for value %q", cfg.Port, bad)
			}
		})
	}
}

func TestLoadRotationMode(t *testing.T) {
	setEnv(t, map[string]string{"MODE": "use", "FLARETUNNEL_MODE": "round-robin"})
	defer clearEnv(t, "MODE", "FLARETUNNEL_MODE")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RotationMode != "round-robin" {
		t.Errorf("RotationMode = %q, want round-robin", cfg.RotationMode)
	}
}

func TestLoadRotationModeUnknownFallsBack(t *testing.T) {
	setEnv(t, map[string]string{"MODE": "use", "FLARETUNNEL_MODE": "banana"})
	defer clearEnv(t, "MODE", "FLARETUNNEL_MODE")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RotationMode != "random" {
		t.Errorf("RotationMode = %q, want default random", cfg.RotationMode)
	}
}

func TestLoadBlacklistLevels(t *testing.T) {
	cases := []struct {
		level string
		file  string
	}{
		{"minimal", "blacklist-minimal.txt"},
		{"full", "blacklist.txt"},
		{"aggressive", "blacklist-aggressive.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.level, func(t *testing.T) {
			setEnv(t, map[string]string{"MODE": "use", "FLARETUNNEL_BLACKLIST": tc.level})
			defer clearEnv(t, "MODE", "FLARETUNNEL_BLACKLIST")
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.BlacklistLevel != tc.level || cfg.BlacklistFile != tc.file {
				t.Errorf("got level=%q file=%q, want level=%q file=%q", cfg.BlacklistLevel, cfg.BlacklistFile, tc.level, tc.file)
			}
		})
	}
}

func TestLoadBlacklistUnknownFallsBack(t *testing.T) {
	setEnv(t, map[string]string{"MODE": "use", "FLARETUNNEL_BLACKLIST": "nope"})
	defer clearEnv(t, "MODE", "FLARETUNNEL_BLACKLIST")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.BlacklistLevel != "minimal" || cfg.BlacklistFile != "blacklist-minimal.txt" {
		t.Errorf("got level=%q file=%q, want minimal fallback", cfg.BlacklistLevel, cfg.BlacklistFile)
	}
}
