// Package config loads and validates the manager's environment configuration.
package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type Mode string

const (
	ModeCreate Mode = "create"
	ModeUse    Mode = "use"
	ModeDelete Mode = "delete"
)

const (
	BlacklistMinimal    = "minimal"
	BlacklistFull       = "full"
	BlacklistAggressive = "aggressive"
)

const (
	RotationRandom     = "random"
	RotationRoundRobin = "round-robin"
)

const (
	DefaultPort           = 8080
	DefaultRotationMode   = RotationRandom
	DefaultBlacklistLevel = BlacklistMinimal
)

var BlacklistLevelFile = map[string]string{
	BlacklistMinimal:    "blacklist-minimal.txt",
	BlacklistFull:       "blacklist.txt",
	BlacklistAggressive: "blacklist-aggressive.txt",
}

type Config struct {
	Mode           Mode
	Port           int
	RotationMode   string
	BlacklistLevel string
	BlacklistFile  string
	AuthProxyBasic string
	CreateAccounts string
	UseAccounts    string
	DeleteAccounts string
}

type authProxyConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// EncodeAuthProxy parses the manager-only JSON secret and returns only the
// Base64 value consumed by FlareTunnel. Secret material is never included in
// returned errors.
func EncodeAuthProxy(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("AUTH_PROXY is required in use mode")
	}
	var auth authProxyConfig
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&auth); err != nil {
		return "", fmt.Errorf("AUTH_PROXY must be valid JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", fmt.Errorf("AUTH_PROXY must contain exactly one JSON object")
	}
	if strings.TrimSpace(auth.Username) == "" {
		return "", fmt.Errorf("AUTH_PROXY username is required")
	}
	if strings.TrimSpace(auth.Password) == "" {
		return "", fmt.Errorf("AUTH_PROXY password is required")
	}
	return base64.StdEncoding.EncodeToString([]byte(auth.Username + ":" + auth.Password)), nil
}

func Load() (*Config, error) {
	cfg := &Config{}
	modeStr := strings.TrimSpace(os.Getenv("MODE"))
	switch Mode(modeStr) {
	case ModeCreate, ModeUse, ModeDelete:
		cfg.Mode = Mode(modeStr)
	default:
		return nil, fmt.Errorf("MODE must be one of 'create', 'use', 'delete' (got %q)", modeStr)
	}

	cfg.Port = DefaultPort
	if raw := strings.TrimSpace(os.Getenv("PORT")); raw != "" {
		if p, err := strconv.Atoi(raw); err == nil && p > 0 && p <= 65535 {
			cfg.Port = p
		}
	}

	cfg.RotationMode = DefaultRotationMode
	if raw := strings.TrimSpace(os.Getenv("FLARETUNNEL_MODE")); raw == RotationRoundRobin || raw == RotationRandom {
		cfg.RotationMode = raw
	}

	cfg.BlacklistLevel = DefaultBlacklistLevel
	cfg.BlacklistFile = BlacklistLevelFile[DefaultBlacklistLevel]
	if raw := strings.TrimSpace(os.Getenv("FLARETUNNEL_BLACKLIST")); raw != "" {
		if file, ok := BlacklistLevelFile[raw]; ok {
			cfg.BlacklistLevel, cfg.BlacklistFile = raw, file
		}
	}

	cfg.CreateAccounts = os.Getenv("CF_CREATE_ACCOUNTS")
	cfg.UseAccounts = os.Getenv("CF_USE_ACCOUNTS")
	cfg.DeleteAccounts = os.Getenv("CF_DELETE_ACCOUNTS")
	if cfg.Mode == ModeUse {
		var err error
		cfg.AuthProxyBasic, err = EncodeAuthProxy(os.Getenv("AUTH_PROXY"))
		if err != nil {
			return nil, err
		}
	}
	return cfg, nil
}
