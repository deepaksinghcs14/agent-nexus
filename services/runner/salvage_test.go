package main

// salvagePush is what stands between a killed/crashed coding session and
// losing every commit it made: runClaudeSession used to `return` on a killed
// session (SESSION_TIMEOUT_MIN SIGKILLs the claude process, which emits no
// parseable JSON) before ever reaching the push, and the caller's deferred
// os.RemoveAll then deletes the clone.
//
// This cannot go red-then-green against the pre-fix code — salvagePush did
// not exist there, so the package simply failed to build. Its job instead is
// locking the two guards a later "simplification" would be tempted to strip:
// the empty-branch check (don't push/report a branch with nothing on it) and
// --force (without it, a retry into the same nexus/<ticket> ref is rejected
// non-fast-forward and its own successful work reports as lost).

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
}

func mustRun(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	out, err := runCmd(t.Context(), dir, nil, name, args...)
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
	return out
}

// newSalvageOrigin creates a bare "origin" repo with one seeded commit on
// main — the shared remote every clone in a test pushes against.
func newSalvageOrigin(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	// -b main: pin the initial branch explicitly. Without it, the bare
	// repo's default branch (and thus HEAD) follows the ambient git
	// installation's init.defaultBranch — "main" on some machines, "master"
	// (or unset) on others. The seed commit below is pushed to "main"
	// regardless, so on a machine defaulting elsewhere, origin's HEAD points
	// at a branch that never receives any commit, and every subsequent
	// `git clone` in this file gets an empty repo with no HEAD to rev-parse.
	mustRun(t, "", "git", "init", "--bare", "-b", "main", origin)

	seed := filepath.Join(root, "seed")
	mustRun(t, "", "git", "clone", origin, seed)
	for _, kv := range [][2]string{{"user.name", "t"}, {"user.email", "t@t.test"}} {
		mustRun(t, seed, "git", "config", kv[0], kv[1])
	}
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	mustRun(t, seed, "git", "add", "-A")
	mustRun(t, seed, "git", "commit", "-m", "seed")
	mustRun(t, seed, "git", "push", "origin", "HEAD:main")
	return origin
}

// cloneAndBranch clones origin into a fresh work dir and checks out branch,
// mirroring runClaudeSession's real clone + `checkout -B branch` sequence —
// including that checkout -B always resets to the freshly-cloned HEAD (main),
// never to a same-named branch that might already exist on origin.
func cloneAndBranch(t *testing.T, origin, branch string) (workDir, baseSHA string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	mustRun(t, "", "git", "clone", origin, work)
	for _, kv := range [][2]string{{"user.name", "t"}, {"user.email", "t@t.test"}} {
		mustRun(t, work, "git", "config", kv[0], kv[1])
	}
	mustRun(t, work, "git", "checkout", "-B", branch)
	sha := strings.TrimSpace(mustRun(t, work, "git", "rev-parse", "HEAD"))
	return work, sha
}

func remoteHasBranch(t *testing.T, work, branch string) bool {
	t.Helper()
	out, err := runCmd(t.Context(), work, nil, "git", "ls-remote", "origin", branch)
	if err != nil {
		t.Fatalf("ls-remote: %v\n%s", err, out)
	}
	return strings.TrimSpace(out) != ""
}

func TestSalvagePushCleanTreeNothingCommitted(t *testing.T) {
	skipIfNoGit(t)
	origin := newSalvageOrigin(t)
	work, baseSHA := cloneAndBranch(t, origin, "nexus/T-1")

	if pushed := salvagePush(work, "nexus/T-1", baseSHA, ""); pushed {
		t.Fatal("salvagePush = true on a clean tree with nothing committed, want false")
	}
	if remoteHasBranch(t, work, "nexus/T-1") {
		t.Fatal("clean-tree salvage must not create a branch on the remote")
	}
}

func TestSalvagePushCommitsAndPushesUncommittedWork(t *testing.T) {
	skipIfNoGit(t)
	origin := newSalvageOrigin(t)
	work, baseSHA := cloneAndBranch(t, origin, "nexus/T-1")

	if err := os.WriteFile(filepath.Join(work, "partial.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write uncommitted file: %v", err)
	}

	if pushed := salvagePush(work, "nexus/T-1", baseSHA, ""); !pushed {
		t.Fatal("salvagePush = false with uncommitted work present, want true")
	}
	if !remoteHasBranch(t, work, "nexus/T-1") {
		t.Fatal("salvage push did not reach the remote")
	}

	// Fetch into a fresh clone and confirm the salvaged file is actually
	// there — not just that some push succeeded.
	verify := filepath.Join(t.TempDir(), "verify")
	mustRun(t, "", "git", "clone", "--branch", "nexus/T-1", origin, verify)
	if _, err := os.Stat(filepath.Join(verify, "partial.go")); err != nil {
		t.Fatalf("salvaged file missing from the pushed branch: %v", err)
	}
}

func TestSalvagePushForceUnblocksRetryOverSameBranch(t *testing.T) {
	skipIfNoGit(t)
	origin := newSalvageOrigin(t)

	work, baseSHA := cloneAndBranch(t, origin, "nexus/T-1")
	if err := os.WriteFile(filepath.Join(work, "first.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write first file: %v", err)
	}
	if pushed := salvagePush(work, "nexus/T-1", baseSHA, ""); !pushed {
		t.Fatal("first salvage push failed")
	}

	// A retry clones fresh from origin (mirrors runClaudeSession's real clone
	// step) and checks out the same ticket branch — checkout -B resets to the
	// clone's own HEAD (main), not origin's now-updated nexus/T-1, so this
	// push is non-fast-forward relative to the first salvage. The
	// orchestrator's "crashed → retry once" always relaunches into this same
	// ref, so --force is what keeps the retry's work from being rejected.
	retryWork, retryBase := cloneAndBranch(t, origin, "nexus/T-1")
	if err := os.WriteFile(filepath.Join(retryWork, "second.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write second file: %v", err)
	}
	if pushed := salvagePush(retryWork, "nexus/T-1", retryBase, ""); !pushed {
		t.Fatal("retry salvage push failed — --force should unblock a non-fast-forward retry")
	}
	retryHead := strings.TrimSpace(mustRun(t, retryWork, "git", "rev-parse", "HEAD"))

	verify := filepath.Join(t.TempDir(), "verify")
	mustRun(t, "", "git", "clone", "--branch", "nexus/T-1", origin, verify)
	remoteHead := strings.TrimSpace(mustRun(t, verify, "git", "rev-parse", "HEAD"))
	if remoteHead != retryHead {
		t.Fatalf("remote head = %s, want the retry's commit %s — the retry's push did not win", remoteHead, retryHead)
	}
	if _, err := os.Stat(filepath.Join(verify, "second.go")); err != nil {
		t.Fatalf("retry's file missing from the remote branch: %v", err)
	}
}
