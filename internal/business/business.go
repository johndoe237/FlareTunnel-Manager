// Package business implements the manager's business logic and orchestration
// of FlareTunnel. It decides what to do per account, delegates the actual
// FlareTunnel operations, and re-checks the real Cloudflare state.
package business

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"flaretunnel-manager/internal/cloudflare"
	"flaretunnel-manager/internal/flaretunnel"
	"flaretunnel-manager/internal/logging"
	"flaretunnel-manager/internal/runtime"
	"flaretunnel-manager/internal/validation"
)

// MaxCreateAttempts is the maximum number of create attempts per account.
const (
	MaxCreateAttempts = 3
	MaxDeleteAttempts = 3
)

// FlareTunnelRunner is the subset of the FlareTunnel runner used by the
// business layer. It is an interface so tests can substitute a stub.
type FlareTunnelRunner interface {
	Create(ctx context.Context, dir, account string, count int) error
	Cleanup(ctx context.Context, dir, account string, count int) error
	List(ctx context.Context, dir string) error
	TunnelArgs(port int, mode, blacklistFile string) []string
	Launch(ctx context.Context, dir string, args []string) error
}

// CloudflareCounter counts the FlareTunnel Workers of an account.
type CloudflareCounter interface {
	CountFlareTunnelWorkers(ctx context.Context) (int, error)
}

// CloudflareFactory builds a CloudflareCounter for an account.
type CloudflareFactory func(acc validation.Account) CloudflareCounter

// RealCloudflare is the production factory backed by the Cloudflare API.
func RealCloudflare(acc validation.Account) CloudflareCounter {
	return cloudflare.New(acc.APIToken, acc.AccountID)
}

// Summary is the short global summary printed at the end of create/delete.
type Summary struct {
	Total      int
	Successful int
	Failed     int
}

// OK reports whether every account succeeded.
func (s Summary) OK() bool { return s.Failed == 0 }

// Create processes MODE=create. Accounts are processed strictly sequentially.
// Each account is reconciled against its target: only Workers following the
// FlareTunnel convention ("flaretunnel-*") are counted, and the missing count
// is recomputed from the real Cloudflare state at every attempt (max 3).
func Create(ctx context.Context, accounts []validation.Account, runner FlareTunnelRunner, rt *runtime.Manager, log *logging.Logger) Summary {
	return CreateWith(ctx, accounts, runner, rt, log, RealCloudflare)
}

// CreateWith is Create with an injectable Cloudflare factory (tests).
func CreateWith(ctx context.Context, accounts []validation.Account, runner FlareTunnelRunner, rt *runtime.Manager, log *logging.Logger, cfFactory CloudflareFactory) Summary {
	sum := Summary{Total: len(accounts)}
	for _, acc := range accounts {
		log.Infof("[%s] Checking Cloudflare state...", acc.Name)
		ok := createAccount(ctx, acc, runner, rt, log, cfFactory)
		if ok {
			sum.Successful++
		} else {
			sum.Failed++
		}
	}
	return sum
}

func createAccount(ctx context.Context, acc validation.Account, runner FlareTunnelRunner, rt *runtime.Manager, log *logging.Logger, cfFactory CloudflareFactory) bool {
	cf := cfFactory(acc)
	target := *acc.TargetWorkers

	dir, err := rt.NewAccountDir(acc.Name)
	if err != nil {
		log.Errorf("[%s] Cannot create runtime directory: %v", acc.Name, err)
		return false
	}
	defer rt.RemoveDir(dir)

	if err := flaretunnel.WriteConfig(dir, acc.Name, acc.APIToken, acc.AccountID, acc.ZoneID); err != nil {
		log.Errorf("[%s] Cannot write flaretunnel.json: %v", acc.Name, err)
		return false
	}

	for attempt := 1; attempt <= MaxCreateAttempts; attempt++ {
		existing, err := cf.CountFlareTunnelWorkers(ctx)
		if err != nil {
			log.Errorf("[%s] Cloudflare state check failed (attempt %d/%d): %v", acc.Name, attempt, MaxCreateAttempts, err)
			if attempt == MaxCreateAttempts {
				return false
			}
			continue
		}
		log.Infof("[%s] Found %d FlareTunnel Workers.", acc.Name, existing)

		if existing >= target {
			log.Infof("[%s] Target: %d. Already reached.", acc.Name, target)
			log.Infof("[%s] SUCCESS", acc.Name)
			return true
		}

		missing := target - existing
		log.Infof("[%s] Target: %d. Missing: %d.", acc.Name, target, missing)
		log.Infof("[%s] Creating %d Workers... (attempt %d/%d)", acc.Name, missing, attempt, MaxCreateAttempts)

		if err := runner.Create(ctx, dir, acc.Name, missing); err != nil {
			log.Errorf("[%s] FlareTunnel create failed (attempt %d/%d): %v", acc.Name, attempt, MaxCreateAttempts, err)
			if attempt == MaxCreateAttempts {
				return false
			}
			continue
		}
	}

	log.Errorf("[%s] FAILED after %d attempts: target not reached.", acc.Name, MaxCreateAttempts)
	return false
}

// Delete processes MODE=delete. target_workers is a deletion count, not a
// desired final population. Every cleanup call is bounded by the real number
// of Workers currently present and the remaining requested count.
func Delete(ctx context.Context, accounts []validation.Account, runner FlareTunnelRunner, rt *runtime.Manager, log *logging.Logger) Summary {
	return DeleteWith(ctx, accounts, runner, rt, log, RealCloudflare)
}

// DeleteWith is Delete with an injectable Cloudflare factory (tests).
func DeleteWith(ctx context.Context, accounts []validation.Account, runner FlareTunnelRunner, rt *runtime.Manager, log *logging.Logger, cfFactory CloudflareFactory) Summary {
	sum := Summary{Total: len(accounts)}
	for _, acc := range accounts {
		log.Infof("[%s] Checking Cloudflare state...", acc.Name)
		ok := deleteAccount(ctx, acc, runner, rt, log, cfFactory)
		if ok {
			sum.Successful++
		} else {
			sum.Failed++
		}
	}
	return sum
}

func deleteAccount(ctx context.Context, acc validation.Account, runner FlareTunnelRunner, rt *runtime.Manager, log *logging.Logger, cfFactory CloudflareFactory) bool {
	cf := cfFactory(acc)
	target := 0
	if acc.TargetWorkers != nil {
		target = *acc.TargetWorkers
	}
	if target == 0 {
		log.Infof("[%s] Target deletions: 0. Nothing to delete.", acc.Name)
		log.Infof("[%s] SUCCESS", acc.Name)
		return true
	}

	dir, err := rt.NewAccountDir(acc.Name)
	if err != nil {
		log.Errorf("[%s] Cannot create runtime directory: %v", acc.Name, err)
		return false
	}
	defer rt.RemoveDir(dir)

	if err := flaretunnel.WriteConfig(dir, acc.Name, acc.APIToken, acc.AccountID, acc.ZoneID); err != nil {
		log.Errorf("[%s] Cannot write flaretunnel.json: %v", acc.Name, err)
		return false
	}

	deleted := 0
	for attempt := 1; attempt <= MaxDeleteAttempts; attempt++ {
		existing, err := cf.CountFlareTunnelWorkers(ctx)
		if err != nil {
			log.Errorf("[%s] Cloudflare state check failed (attempt %d/%d): %v", acc.Name, attempt, MaxDeleteAttempts, err)
			if attempt == MaxDeleteAttempts {
				return false
			}
			continue
		}
		log.Infof("[%s] Found %d FlareTunnel Workers.", acc.Name, existing)

		remaining := target - deleted
		if remaining <= 0 {
			log.Infof("[%s] Target deletion count %d reached.", acc.Name, target)
			log.Infof("[%s] SUCCESS", acc.Name)
			return true
		}
		if existing == 0 {
			log.Infof("[%s] No FlareTunnel Workers remain; requested deletion was %d, completed %d.", acc.Name, target, deleted)
			// The real state is authoritative: do not attempt to delete nonexistent Workers.
			log.Infof("[%s] SUCCESS", acc.Name)
			return true
		}

		count := remaining
		if existing < count {
			count = existing
		}
		log.Infof("[%s] Target deletions: %d. Remaining: %d. Requesting bounded cleanup of %d.", acc.Name, target, remaining, count)
		log.Infof("[%s] Cleaning up FlareTunnel Workers... (attempt %d/%d)", acc.Name, attempt, MaxDeleteAttempts)
		if err := runner.Cleanup(ctx, dir, acc.Name, count); err != nil {
			log.Errorf("[%s] FlareTunnel cleanup failed (attempt %d/%d): %v", acc.Name, attempt, MaxDeleteAttempts, err)
			if attempt == MaxDeleteAttempts {
				return false
			}
			continue
		}

		after, err := cf.CountFlareTunnelWorkers(ctx)
		if err != nil {
			log.Errorf("[%s] Post-cleanup Cloudflare state check failed (attempt %d/%d): %v", acc.Name, attempt, MaxDeleteAttempts, err)
			if attempt == MaxDeleteAttempts {
				return false
			}
			continue
		}
		removed := existing - after
		if removed > count {
			removed = count
		}
		if removed > 0 {
			deleted += removed
		}
		log.Infof("[%s] Verification: %d FlareTunnel Workers remaining; deleted %d/%d requested.", acc.Name, after, deleted, target)
		if deleted >= target || after == 0 {
			log.Infof("[%s] SUCCESS", acc.Name)
			return true
		}
	}

	log.Errorf("[%s] FAILED after %d attempts: deleted %d/%d requested.", acc.Name, MaxDeleteAttempts, deleted, target)
	return false
}

// Use processes MODE=use. It is a pure preparation/bootstrap layer: it never
// creates Workers. The bootstrap order is:
//
//	credentials -> flaretunnel.json -> FlareTunnel list -> flaretunnel_endpoints.json
//	-> delete flaretunnel.json -> exec FlareTunnel tunnel
//
// flaretunnel_endpoints.json and the blacklist files are kept: they are
// required by the tunnel mode after the exec.
func Use(ctx context.Context, accounts []validation.Account, runner FlareTunnelRunner, rt *runtime.Manager, log *logging.Logger, port int, rotationMode, blacklistFile string) error {
	dir, err := rt.UseDir()
	if err != nil {
		return err
	}

	// 1. Write flaretunnel.json with all accounts (temporary bootstrap file).
	cfg := flaretunnel.Config{Accounts: make([]flaretunnel.Account, 0, len(accounts))}
	for _, acc := range accounts {
		cfg.Accounts = append(cfg.Accounts, flaretunnel.Account{
			Name:      acc.Name,
			APIToken:  acc.APIToken,
			AccountID: acc.AccountID,
			ZoneID:    acc.ZoneID,
		})
	}
	cfgPath := filepath.Join(dir, flaretunnel.ConfigFile)
	if err := writeConfigFile(cfgPath, cfg); err != nil {
		return fmt.Errorf("cannot write flaretunnel.json: %w", err)
	}

	// 2. Use FlareTunnel to list the account's Workers. FlareTunnel filters on
	//    the "flaretunnel-*" convention itself and writes
	//    flaretunnel_endpoints.json in its working directory.
	log.Infof("Listing FlareTunnel Workers via FlareTunnel...")
	if err := runner.List(ctx, dir); err != nil {
		_ = os.Remove(cfgPath)
		return fmt.Errorf("flaretunnel list failed: %w", err)
	}

	// 3. Load the endpoints produced by FlareTunnel and verify at least one is
	//    usable.
	eps, err := flaretunnel.ReadEndpoints(filepath.Join(dir, flaretunnel.EndpointsFile))
	if err != nil {
		_ = os.Remove(cfgPath)
		return fmt.Errorf("cannot read flaretunnel_endpoints.json: %w", err)
	}
	if len(eps) == 0 {
		_ = os.Remove(cfgPath)
		return fmt.Errorf("no usable FlareTunnel endpoint found")
	}
	log.Infof("Found %d FlareTunnel endpoint(s).", len(eps))

	// 4. Delete the temporary credentials file before the exec.
	if err := os.Remove(cfgPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove temporary flaretunnel.json: %w", err)
	}
	log.Infof("Temporary flaretunnel.json removed.")

	// 5. Verify the blacklist file exists in the directory FlareTunnel expects.
	if _, err := os.Stat(blacklistFile); err != nil {
		return fmt.Errorf("blacklist file %s not found: %v", blacklistFile, err)
	}

	// 6. Launch FlareTunnel in tunnel mode via exec. On success the manager
	//    process is replaced by FlareTunnel, which becomes the main process.
	log.Infof("Launching FlareTunnel tunnel (port %d, mode %s, blacklist %s).", port, rotationMode, filepath.Base(blacklistFile))
	return runner.Launch(ctx, dir, runner.TunnelArgs(port, rotationMode, blacklistFile))
}

func writeConfigFile(path string, cfg flaretunnel.Config) error {
	data, err := jsonMarshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
