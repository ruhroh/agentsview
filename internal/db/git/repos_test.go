package git

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// initBareRepo runs `git init -b main` at root and configures a
// deterministic identity so commit creation never prompts. Returns the
// repo path.
func initBareRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-q", "-b", "main")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "commit.gpgsign", "false")
	return repo
}

// mkdirIn creates rel under root and returns the absolute path.
func mkdirIn(t *testing.T, root, rel string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
	return p
}

// canonAll resolves each path through filepath.EvalSymlinks (falling back
// to the original on error) and returns a sorted copy. Needed because
// `git rev-parse --show-toplevel` returns canonical paths, which on macOS
// expand /var to /private/var.
func canonAll(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			out[i] = r
		} else {
			out[i] = p
		}
	}
	sort.Strings(out)
	return out
}

func TestDiscoverRepos_FindsRootAndFiltersMissing(t *testing.T) {
	skipIfNoGit(t)
	repoA := initBareRepo(t)
	sub := mkdirIn(t, repoA, "subdir")
	outside := t.TempDir()

	got := DiscoverRepos([]string{sub, outside})
	want := []string{repoA}
	if !reflect.DeepEqual(canonAll(got), canonAll(want)) {
		t.Fatalf("DiscoverRepos = %v, want %v", got, want)
	}
}

func TestDiscoverRepos_Dedup(t *testing.T) {
	skipIfNoGit(t)
	repoA := initBareRepo(t)
	sub1 := mkdirIn(t, repoA, "sub1")
	sub2 := mkdirIn(t, repoA, "sub2/deeper")

	got := DiscoverRepos([]string{sub1, sub2, repoA})
	if len(got) != 1 {
		t.Fatalf("DiscoverRepos = %v, want exactly one entry "+
			"(dedup)", got)
	}
	if !reflect.DeepEqual(canonAll(got), canonAll([]string{repoA})) {
		t.Fatalf("DiscoverRepos = %v, want %v", got, repoA)
	}
}

func TestDiscoverRepos_EmptyInputReturnsEmptySlice(t *testing.T) {
	got := DiscoverRepos(nil)
	if got == nil || len(got) != 0 {
		t.Fatalf("DiscoverRepos(nil) = %v, want non-nil empty slice",
			got)
	}
	got = DiscoverRepos([]string{})
	if got == nil || len(got) != 0 {
		t.Fatalf("DiscoverRepos([]) = %v, want non-nil empty slice",
			got)
	}
}

// TestDiscoverRepos_LinkedWorktreeResolves covers the regression flagged
// by code review: linked worktrees use a `.git` FILE (not directory)
// that points at the parent gitdir. `git rev-parse --show-toplevel`
// resolves these, so worktree cwds must contribute a repo root rather
// than being silently dropped.
func TestDiscoverRepos_LinkedWorktreeResolves(t *testing.T) {
	skipIfNoGit(t)
	repo := initBareRepo(t)
	// `git worktree add` requires at least one commit in the source
	// repo, so seed one before linking.
	writeFile(t, repo, "seed.txt", []byte("seed\n"))
	commitAs(t, repo, "test@example.com", "Test User", "seed")

	worktreeRoot := filepath.Join(t.TempDir(), "wt")
	gitRun(t, repo, nil,
		"worktree", "add", "-b", "feature", worktreeRoot,
	)

	got := DiscoverRepos([]string{worktreeRoot})
	if len(got) != 1 {
		t.Fatalf("DiscoverRepos = %v, want one worktree root", got)
	}
	if !reflect.DeepEqual(
		canonAll(got),
		canonAll([]string{worktreeRoot}),
	) {
		t.Fatalf("DiscoverRepos = %v, want %v "+
			"(worktree path)", got, worktreeRoot)
	}
}

// TestDiscoverRepos_MissingCwdSkipped confirms that a cwd whose path is
// completely outside any git repo (and which does not exist on disk)
// produces no false-positive root.
func TestDiscoverRepos_MissingCwdSkipped(t *testing.T) {
	skipIfNoGit(t)
	missing := filepath.Join(t.TempDir(), "no", "such", "path")

	got := DiscoverRepos([]string{missing})
	if len(got) != 0 {
		t.Fatalf("DiscoverRepos = %v, want empty (missing path)", got)
	}
}
