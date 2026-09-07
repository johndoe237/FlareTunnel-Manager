// Command flaretunnel-manager is the orchestration layer for FlareTunnel.
//
// It prepares data, queries Cloudflare when needed, decides what to do, and
// delegates the actual FlareTunnel operations to the FlareTunnel binary.
// It never re-implements FlareTunnel functionality.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"flaretunnel-manager/internal/business"
	"flaretunnel-manager/internal/config"
	"flaretunnel-manager/internal/flaretunnel"
	"flaretunnel-manager/internal/logging"
	"flaretunnel-manager/internal/runtime"
	"flaretunnel-manager/internal/validation"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		return 2
	}

	log := logging.New()

	rt, err := runtime.New("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Runtime error: %v\n", err)
		return 2
	}
	// Best-effort cleanup of the whole runtime tree at process exit so that no
	// temporary credential file is left on disk. In MODE=use this defer never
	// runs on success because the process is replaced by FlareTunnel (exec).
	defer rt.CleanupAll()

	runner := flaretunnel.New(os.Getenv("FLARETUNNEL_BINARY"))

	ctx := context.Background()

	switch cfg.Mode {
	case config.ModeCreate:
		accounts, err := validation.Validate(cfg.CreateAccounts, true)
		if err != nil {
			log.Errorf("Invalid CF_CREATE_ACCOUNTS: %v", err)
			return 2
		}
		log.Infof("MODE=create: %d account(s) to process.", len(accounts))
		sum := business.Create(ctx, accounts, runner, rt, log)
		printSummary(sum)
		if !sum.OK() {
			return 1
		}
		return 0

	case config.ModeDelete:
		accounts, err := validation.Validate(cfg.DeleteAccounts, true)
		if err != nil {
			log.Errorf("Invalid CF_DELETE_ACCOUNTS: %v", err)
			return 2
		}
		log.Infof("MODE=delete: %d account(s) to process.", len(accounts))
		sum := business.Delete(ctx, accounts, runner, rt, log)
		printSummary(sum)
		if !sum.OK() {
			return 1
		}
		return 0

	case config.ModeUse:
		accounts, err := validation.Validate(cfg.UseAccounts, false)
		if err != nil {
			log.Errorf("Invalid CF_USE_ACCOUNTS: %v", err)
			return 2
		}
		// Blacklists are shipped by the image; callers select only the level.
		blacklistPath := filepath.Join("/opt/flaretunnel", cfg.BlacklistFile)
		log.Infof("MODE=use: bootstrapping %d account(s).", len(accounts))
		if err := business.Use(ctx, accounts, runner, rt, log, cfg.Port, cfg.RotationMode, blacklistPath, cfg.AuthProxyBasic); err != nil {
			log.Errorf("Bootstrap failed: %v", err)
			return 1
		}
		return 0
	}

	return 2
}

func printSummary(s business.Summary) {
	if s.OK() {
		fmt.Printf("Operation completed.\nAccounts: %d\nSuccessful: %d\nFailed: %d\n", s.Total, s.Successful, s.Failed)
	} else {
		fmt.Printf("Operation completed with errors.\nAccounts: %d\nSuccessful: %d\nFailed: %d\n", s.Total, s.Successful, s.Failed)
	}
}
