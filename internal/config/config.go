// Package config loads and validates the manager's environment configuration.
//
// Values with a predefined set of allowed values fall back to their default
// when an unknown value is provided. The program does not fail solely because
// an optional variable carries an unknown value when a default fallback exists.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Mode identifies the operational mode of the manager.
type Mode string

const (
	ModeCreate Mode = "create"
	ModeUse    Mode = "use"
	ModeDelete Mode = "delete"
)

// Blacklist levels exposed by FlareTunnel.
const (
	BlacklistMinimal    = "minimal"
	BlacklistFull       = "full"
	BlacklistAggressive = "aggressive"
)

// Rotation modes supported by FlareTunnel.
const (
	RotationRandom     = "random"
	RotationRoundRobin = "round-robin"
)

// Defaults.
const (
	DefaultPort           = 8080
	DefaultRotationMode   = RotationRandom
	DefaultBlacklistLevel = BlacklistMinimal
)

// BlacklistLevelFile maps a blacklist level to the FlareTunnel blacklist file.
// These are the exact filenames shipped by FlareTunnel.
var BlacklistLevelFile = map[string]string{
	BlacklistMinimal:    "blacklist-minimal.txt",
	BlacklistFull:       "blacklist.txt",
	BlacklistAggressive: "blacklist-aggressive.txt",
}

// Config is the validated runtime configuration.
type Config struct {
	Mode           Mode
	Port           int
	RotationMode   string
	BlacklistLevel string
	BlacklistFile  string
	CreateAccounts string // raw JSON from CF_CREATE_ACCOUNTS
	UseAccounts    string // raw JSON from CF_USE_ACCOUNTS
	DeleteAccounts string // raw JSON from CF_DELETE_ACCOUNTS
}

// Load reads and validates the environment variables.
func Load() (*Config, error) {
	cfg := &Config{}

	// MODE is required and has no default: an unknown value is a hard error.
	modeStr := strings.TrimSpace(os.Getenv("MODE"))
	switch Mode(modeStr) {
	case ModeCreate, ModeUse, ModeDelete:
		cfg.Mode = Mode(modeStr)
	default:
		return nil, fmt.Errorf("MODE must be one of 'create', 'use', 'delete' (got %q)", modeStr)
	}

	// PORT: default 8080. A non-numeric or out-of-range value falls back to the
	// default rather than failing the program.
	cfg.Port = DefaultPort
	if raw := strings.TrimSpace(os.Getenv("PORT")); raw != "" {
		if p, err := strconv.Atoi(raw); err == nil && p > 0 && p <= 65535 {
			cfg.Port = p
		} else {
			cfg.Port = DefaultPort
		}
	}

	// FLARETUNNEL_MODE: default "random". Unknown value -> default.
	cfg.RotationMode = DefaultRotationMode
	if raw := strings.TrimSpace(os.Getenv("FLARETUNNEL_MODE")); raw != "" {
		switch raw {
		case RotationRandom, RotationRoundRobin:
			cfg.RotationMode = raw
		default:
			cfg.RotationMode = DefaultRotationMode
		}
	}

	// FLARETUNNEL_BLACKLIST: default "minimal". Unknown value -> default.
	cfg.BlacklistLevel = DefaultBlacklistLevel
	cfg.BlacklistFile = BlacklistLevelFile[DefaultBlacklistLevel]
	if raw := strings.TrimSpace(os.Getenv("FLARETUNNEL_BLACKLIST")); raw != "" {
		if file, ok := BlacklistLevelFile[raw]; ok {
			cfg.BlacklistLevel = raw
			cfg.BlacklistFile = file
		} else {
			cfg.BlacklistLevel = DefaultBlacklistLevel
			cfg.BlacklistFile = BlacklistLevelFile[DefaultBlacklistLevel]
		}
	}

	cfg.CreateAccounts = os.Getenv("CF_CREATE_ACCOUNTS")
	cfg.UseAccounts = os.Getenv("CF_USE_ACCOUNTS")
	cfg.DeleteAccounts = os.Getenv("CF_DELETE_ACCOUNTS")

	return cfg, nil
}
