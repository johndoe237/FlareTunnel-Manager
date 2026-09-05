package business

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"flaretunnel-manager/internal/flaretunnel"
	"flaretunnel-manager/internal/logging"
	"flaretunnel-manager/internal/runtime"
	"flaretunnel-manager/internal/validation"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

type fakeCounter struct {
	counts []int // successive counts returned by CountFlareTunnelWorkers
	err    error
	mu     sync.Mutex
}

func (f *fakeCounter) CountFlareTunnelWorkers(_ context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return 0, f.err
	}
	if len(f.counts) == 0 {
		return 0, nil
	}
	n := f.counts[0]
	f.counts = f.counts[1:]
	return n, nil
}

type fakeRunner struct {
	mu        sync.Mutex
	creates   []createCall
	cleanups  []cleanupCall
	lists     int
	launches  []launchCall
	launchErr error
}

type createCall struct {
	account string
	count   int
}
type cleanupCall struct {
	account string
	count   int
}
type launchCall struct {
	args []string
}

func (f *fakeRunner) Create(_ context.Context, _ string, account string, count int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates = append(f.creates, createCall{account, count})
	return nil
}
func (f *fakeRunner) Cleanup(_ context.Context, _ string, account string, count int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanups = append(f.cleanups, cleanupCall{account: account, count: count})
	return nil
}
func (f *fakeRunner) List(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists++
	return nil
}
func (f *fakeRunner) TunnelArgs(port int, mode, blacklistFile string) []string {
	return []string{"tunnel", "--port", itoa(port), "--mode", mode, "--blacklist", blacklistFile}
}
func (f *fakeRunner) Launch(_ context.Context, _ string, args []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.launches = append(f.launches, launchCall{args})
	return f.launchErr
}

// factory returns a counter whose counts are consumed in order.
func factory(counts []int) CloudflareFactory {
	return func(validation.Account) CloudflareCounter { return &fakeCounter{counts: counts} }
}

// factoryByAccount returns per-account counters keyed by account name.
func factoryByAccount(per map[string][]int) CloudflareFactory {
	return func(acc validation.Account) CloudflareCounter {
		return &fakeCounter{counts: per[acc.Name]}
	}
}

func newTestManager(t *testing.T) *runtime.Manager {
	t.Helper()
	m, err := runtime.New(filepath.Join(t.TempDir(), "rt"))
	if err != nil {
		t.Fatalf("runtime.New: %v", err)
	}
	return m
}

func quietLogger() *logging.Logger { return logging.New() }

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestCreateCountsOnlyFlareTunnelWorkersAndComputesMissing(t *testing.T) {
	// 150 total Cloudflare workers, 80 FlareTunnel, target 100 => missing 20.
	acc := []validation.Account{{Name: "acc-a", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(100)}}
	runner := &fakeRunner{}
	sum := CreateWith(context.Background(), acc, runner, newTestManager(t), quietLogger(), factory([]int{80, 100}))

	if sum.Total != 1 || sum.Successful != 1 || sum.Failed != 0 {
		t.Fatalf("summary = %+v, want 1/1/0", sum)
	}
	if len(runner.creates) != 1 || runner.creates[0].count != 20 {
		t.Errorf("creates = %+v, want one create of 20 (80 -> 100, not 150-100)", runner.creates)
	}
	if runner.creates[0].account != "acc-a" {
		t.Errorf("create account = %q, want acc-a", runner.creates[0].account)
	}
}

func TestCreateRecomputesMissingEachAttempt(t *testing.T) {
	// Attempt 1: 80 existing -> create 20 -> now 94
	// Attempt 2: 94 existing -> create 6  -> now 100
	// Attempt 3: 100 existing -> success (no create)
	acc := []validation.Account{{Name: "acc-a", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(100)}}
	runner := &fakeRunner{}
	sum := CreateWith(context.Background(), acc, runner, newTestManager(t), quietLogger(), factory([]int{80, 94, 100}))

	if sum.Successful != 1 || sum.Failed != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	if len(runner.creates) != 2 {
		t.Fatalf("creates = %+v, want exactly 2 (20 then 6)", runner.creates)
	}
	if runner.creates[0].count != 20 || runner.creates[1].count != 6 {
		t.Errorf("create counts = %d,%d want 20,6", runner.creates[0].count, runner.creates[1].count)
	}
}

func TestCreateMaxThreeAttemptsThenFail(t *testing.T) {
	// Never reaches target: 80, 90, 95 after creates.
	acc := []validation.Account{{Name: "acc-a", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(100)}}
	runner := &fakeRunner{}
	sum := CreateWith(context.Background(), acc, runner, newTestManager(t), quietLogger(), factory([]int{80, 90, 95}))

	if sum.Successful != 0 || sum.Failed != 1 {
		t.Fatalf("summary = %+v, want failed", sum)
	}
	if len(runner.creates) != 3 {
		t.Errorf("creates = %d, want exactly 3 attempts", len(runner.creates))
	}
}

func TestCreateTargetAlreadyReachedCreatesNothing(t *testing.T) {
	acc := []validation.Account{{Name: "acc-a", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(100)}}
	runner := &fakeRunner{}
	sum := CreateWith(context.Background(), acc, runner, newTestManager(t), quietLogger(), factory([]int{100}))

	if sum.Successful != 1 {
		t.Fatalf("summary = %+v, want success", sum)
	}
	if len(runner.creates) != 0 {
		t.Errorf("creates = %+v, want no create when target reached", runner.creates)
	}
}

func TestCreateContinuesToNextAccountAfterFailure(t *testing.T) {
	// acc-a fails (never reaches target), acc-b succeeds.
	accounts := []validation.Account{
		{Name: "acc-a", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(100)},
		{Name: "acc-b", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(5)},
	}
	runner := &fakeRunner{}
	// acc-a: 80,90,95 (3 attempts, fail). acc-b: 5 (already reached).
	sum := CreateWith(context.Background(), accounts, runner, newTestManager(t), quietLogger(), factory([]int{80, 90, 95, 5}))

	if sum.Total != 2 || sum.Successful != 1 || sum.Failed != 1 {
		t.Fatalf("summary = %+v, want 2/1/1", sum)
	}
	if len(runner.creates) != 3 {
		t.Errorf("creates = %d, want 3 (all for acc-a)", len(runner.creates))
	}
	for _, c := range runner.creates {
		if c.account != "acc-a" {
			t.Errorf("create for %q, want all creates on acc-a", c.account)
		}
	}
}

func TestCreateSequentialNoParallelism(t *testing.T) {
	// The loop is sequential by construction; this test asserts the runner is
	// never given a second account before the first finished (single-threaded
	// order recorded in the stub).
	accounts := []validation.Account{
		{Name: "acc-a", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(10)},
		{Name: "acc-b", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(10)},
	}
	runner := &fakeRunner{}
	CreateWith(context.Background(), accounts, runner, newTestManager(t), quietLogger(), factory([]int{0, 10, 0, 10}))
	// acc-a: 0 -> create 10 -> 10 success. acc-b: 0 -> create 10 -> 10 success.
	if len(runner.creates) != 2 {
		t.Fatalf("creates = %+v", runner.creates)
	}
	if runner.creates[0].account != "acc-a" || runner.creates[1].account != "acc-b" {
		t.Errorf("sequential order violated: %+v", runner.creates)
	}
}

func TestCreateRemovesAccountRuntimeDir(t *testing.T) {
	acc := []validation.Account{{Name: "acc-a", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(5)}}
	m := newTestManager(t)
	CreateWith(context.Background(), acc, &fakeRunner{}, m, quietLogger(), factory([]int{5}))

	entries, err := os.ReadDir(m.Base)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("runtime dir not cleaned: %v", entries)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestDeleteRunsCleanupWithYesAndVerifies(t *testing.T) {
	acc := []validation.Account{{Name: "acc-a", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(5)}}
	runner := &fakeRunner{}
	// before: 5 workers; after cleanup: 0.
	sum := DeleteWith(context.Background(), acc, runner, newTestManager(t), quietLogger(), factory([]int{5, 0}))

	if sum.Successful != 1 || sum.Failed != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	if len(runner.cleanups) != 1 || runner.cleanups[0].account != "acc-a" {
		t.Errorf("cleanups = %+v, want one cleanup of acc-a", runner.cleanups)
	}
	if runner.cleanups[0].count != 5 {
		t.Errorf("cleanup count = %d, want 5", runner.cleanups[0].count)
	}
}

func TestDeleteBoundsCleanupToExistingWorkers(t *testing.T) {
	acc := []validation.Account{{Name: "acc-a", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(20)}}
	runner := &fakeRunner{}
	sum := DeleteWith(context.Background(), acc, runner, newTestManager(t), quietLogger(), factory([]int{5, 0}))
	if !sum.OK() || len(runner.cleanups) != 1 || runner.cleanups[0].count != 5 {
		t.Fatalf("summary=%+v cleanups=%+v, want one bounded cleanup of 5", sum, runner.cleanups)
	}
}

func TestDeleteRecomputesRemainingAfterPartialProgress(t *testing.T) {
	acc := []validation.Account{{Name: "acc-a", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(20)}}
	runner := &fakeRunner{}
	// Before/after: 50->40 (10 removed), then 40->30 (10 removed).
	sum := DeleteWith(context.Background(), acc, runner, newTestManager(t), quietLogger(), factory([]int{50, 40, 40, 30}))
	if !sum.OK() || len(runner.cleanups) != 2 {
		t.Fatalf("summary=%+v cleanups=%+v, want two attempts", sum, runner.cleanups)
	}
	if runner.cleanups[0].count != 20 || runner.cleanups[1].count != 10 {
		t.Errorf("cleanup counts=%+v, want 20 then 10", runner.cleanups)
	}
}

func TestDeleteSkipsWhenAlreadyZero(t *testing.T) {
	acc := []validation.Account{{Name: "acc-a", APIToken: "t", AccountID: "i"}}
	runner := &fakeRunner{}
	sum := DeleteWith(context.Background(), acc, runner, newTestManager(t), quietLogger(), factory([]int{0}))

	if sum.Successful != 1 {
		t.Fatalf("summary = %+v, want success without deletion", sum)
	}
	if len(runner.cleanups) != 0 {
		t.Errorf("cleanups = %+v, want no cleanup when already zero", runner.cleanups)
	}
}

func TestDeleteVerificationFailsWhenWorkersRemain(t *testing.T) {
	acc := []validation.Account{{Name: "acc-a", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(2)}}
	runner := &fakeRunner{}
	// before: 3; one removed, then no further progress for all retries -> FAILED.
	sum := DeleteWith(context.Background(), acc, runner, newTestManager(t), quietLogger(), factory([]int{3, 2, 2, 2, 2, 2, 2}))

	if sum.Successful != 0 || sum.Failed != 1 {
		t.Fatalf("summary = %+v, want failed", sum)
	}
}

func TestDeleteContinuesToNextAccount(t *testing.T) {
	accounts := []validation.Account{
		{Name: "acc-a", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(2)},
		{Name: "acc-b", APIToken: "t", AccountID: "i", TargetWorkers: intPtr(0)},
	}
	runner := &fakeRunner{}
	// acc-a fails (cleanup leaves workers), acc-b already zero.
	sum := DeleteWith(context.Background(), accounts, runner, newTestManager(t), quietLogger(), factoryByAccount(map[string][]int{
		"acc-a": {3, 2, 2, 2, 2, 2, 2},
		"acc-b": {0},
	}))

	if sum.Total != 2 || sum.Successful != 1 || sum.Failed != 1 {
		t.Fatalf("summary = %+v, want 2/1/1", sum)
	}
	if len(runner.cleanups) != 3 || runner.cleanups[0].account != "acc-a" || runner.cleanups[2].account != "acc-a" {
		t.Errorf("cleanups = %+v, want three retries for acc-a", runner.cleanups)
	}
}

// ---------------------------------------------------------------------------
// Use
// ---------------------------------------------------------------------------

func TestUsePreparesEndpointsRemovesCredentialsAndLaunches(t *testing.T) {
	m := newTestManager(t)
	dir, err := m.UseDir()
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the endpoints file that FlareTunnel list would have produced.
	eps := []flaretunnel.Endpoint{{Name: "flaretunnel-1-x", URL: "https://x.sub.workers.dev", ID: "flaretunnel-1-x", AccountID: "i", ConfigAccountName: "acc-a"}}
	if err := flaretunnel.WriteEndpoints(filepath.Join(dir, "flaretunnel_endpoints.json"), eps); err != nil {
		t.Fatal(err)
	}

	accounts := []validation.Account{{Name: "acc-a", APIToken: "tok-a", AccountID: "id-a"}}
	runner := &fakeRunner{}
	blacklist := filepath.Join(dir, "blacklist-minimal.txt")
	if err := os.WriteFile(blacklist, []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = Use(context.Background(), accounts, runner, m, quietLogger(), 9090, "random", blacklist)
	if err != nil {
		t.Fatalf("Use() error = %v", err)
	}

	// flaretunnel.json must have been removed before launch.
	if _, statErr := os.Stat(filepath.Join(dir, "flaretunnel.json")); !os.IsNotExist(statErr) {
		t.Errorf("flaretunnel.json still present after bootstrap")
	}
	// flaretunnel_endpoints.json must be kept.
	if _, statErr := os.Stat(filepath.Join(dir, "flaretunnel_endpoints.json")); statErr != nil {
		t.Errorf("flaretunnel_endpoints.json must be kept for tunnel mode: %v", statErr)
	}

	if len(runner.launches) != 1 {
		t.Fatalf("launches = %+v, want exactly one exec", runner.launches)
	}
	args := strings.Join(runner.launches[0].args, " ")
	if !strings.Contains(args, "--port 9090") {
		t.Errorf("launch args missing port: %s", args)
	}
	if !strings.Contains(args, "--mode random") {
		t.Errorf("launch args missing rotation mode: %s", args)
	}
	if !strings.Contains(args, "--blacklist "+blacklist) {
		t.Errorf("launch args missing blacklist: %s", args)
	}
}

func TestUseFailsWhenNoEndpoint(t *testing.T) {
	m := newTestManager(t)
	dir, _ := m.UseDir()
	accounts := []validation.Account{{Name: "acc-a", APIToken: "tok-a", AccountID: "id-a"}}
	runner := &fakeRunner{}
	blacklist := filepath.Join(dir, "blacklist-minimal.txt")
	os.WriteFile(blacklist, []byte("# t\n"), 0o644)

	// No endpoints file: list "succeeds" but no endpoints -> error.
	if err := Use(context.Background(), accounts, runner, m, quietLogger(), 8080, "random", blacklist); err == nil {
		t.Fatal("expected error when no endpoint is usable")
	}
	// Credentials must be cleaned even on failure.
	if _, statErr := os.Stat(filepath.Join(dir, "flaretunnel.json")); !os.IsNotExist(statErr) {
		t.Errorf("flaretunnel.json left on disk after failed bootstrap")
	}
}

func TestUseListFailureCleansCredentials(t *testing.T) {
	m := newTestManager(t)
	dir, _ := m.UseDir()
	accounts := []validation.Account{{Name: "acc-a", APIToken: "tok-a", AccountID: "id-a"}}
	blacklist := filepath.Join(dir, "blacklist-minimal.txt")
	os.WriteFile(blacklist, []byte("# t\n"), 0o644)

	// Make List fail by pointing the runner at a missing binary.
	badRunner := &failingRunner{}
	if err := Use(context.Background(), accounts, badRunner, m, quietLogger(), 8080, "random", blacklist); err == nil {
		t.Fatal("expected error when flaretunnel list fails")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "flaretunnel.json")); !os.IsNotExist(statErr) {
		t.Errorf("flaretunnel.json left on disk after list failure")
	}
}

type failingRunner struct{}

func (f *failingRunner) Create(context.Context, string, string, int) error  { return nil }
func (f *failingRunner) Cleanup(context.Context, string, string, int) error { return nil }
func (f *failingRunner) List(context.Context, string) error                 { return os.ErrNotExist }
func (f *failingRunner) TunnelArgs(port int, mode, blacklistFile string) []string {
	return []string{"tunnel", "--port", itoa(port), "--mode", mode, "--blacklist", blacklistFile}
}
func (f *failingRunner) Launch(context.Context, string, []string) error { return nil }

func itoa(i int) string { return strconv.Itoa(i) }

func intPtr(i int) *int { return &i }
