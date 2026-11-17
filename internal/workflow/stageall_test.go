package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestStageAllAndUnstageAll(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, ".gitignore"), "*.log\n")
	write(t, filepath.Join(dir, "mod.txt"), "1\n")
	write(t, filepath.Join(dir, "gone.txt"), "bye\n")
	write(t, filepath.Join(dir, "mixed.txt"), "1\n")
	write(t, filepath.Join(dir, "old.txt"), "a\nb\nc\nd\n")
	e.commit(dir, "Initial")

	write(t, filepath.Join(dir, "mod.txt"), "2\n")
	os.Remove(filepath.Join(dir, "gone.txt"))
	write(t, filepath.Join(dir, "mixed.txt"), "staged\n")
	e.git(dir, "add", "mixed.txt")
	write(t, filepath.Join(dir, "mixed.txt"), "worktree\n")
	e.git(dir, "mv", "old.txt", "new.txt")
	// Paths that would be pathspec magic or need quoting.
	special := []string{"new file.txt", "中文.txt", "a*b.txt", ":colon.txt", "-dash.txt", "dir/deep.txt"}
	for _, p := range special {
		write(t, filepath.Join(dir, p), p+"\n")
	}
	write(t, filepath.Join(dir, "debug.log"), "ignored\n")
	r, _ := e.open(dir)
	ctx := context.Background()

	snap, res, err := r.StageAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 0 || len(snap.Unstaged)+len(snap.Untracked) != 0 {
		t.Fatalf("left %v %v skipped %v", paths(snap.Unstaged), paths(snap.Untracked), paths(res.Skipped))
	}
	for _, p := range append(special, "mod.txt", "mixed.txt", "gone.txt") {
		find(t, snap.Staged, p)
	}
	if got := e.git(dir, "show", ":mixed.txt"); got != "worktree\n" {
		t.Fatalf("mixed file staged as %q", got)
	}
	if out := e.git(dir, "ls-files", "--", "debug.log"); out != "" {
		t.Fatal("ignored file staged")
	}
	if ch := find(t, snap.Staged, "new.txt"); ch.OrigPath != "old.txt" {
		t.Fatalf("rename %+v", ch)
	}
	// Nothing was committed.
	if n := strings.Count(e.git(dir, "rev-list", "HEAD"), "\n"); n != 1 {
		t.Fatalf("%d commits", n)
	}
	_, _, err = r.StageAll(ctx)
	wantCode(t, err, CodeNothingToStage)

	snap, err = r.UnstageAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Staged) != 0 {
		t.Fatalf("still staged %v", paths(snap.Staged))
	}
	// The working tree is as it was: content, deletion and new files.
	if readFile(t, filepath.Join(dir, "mixed.txt")) != "worktree\n" || readFile(t, filepath.Join(dir, "mod.txt")) != "2\n" {
		t.Fatal("working tree changed")
	}
	if _, err := os.Stat(filepath.Join(dir, "gone.txt")); !os.IsNotExist(err) {
		t.Fatal("deleted file restored")
	}
	for _, p := range special {
		if readFile(t, filepath.Join(dir, p)) != p+"\n" {
			t.Fatalf("%s changed", p)
		}
		find(t, snap.Untracked, p)
	}
	find(t, snap.Unstaged, "gone.txt")
	find(t, snap.Unstaged, "old.txt")
	find(t, snap.Untracked, "new.txt")
	_, err = r.UnstageAll(ctx)
	wantCode(t, err, CodeNothingStaged)
}

func TestStageAllAndUnstageAllBeforeFirstCommit(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	write(t, filepath.Join(dir, "b/c.txt"), "c\n")
	r, _ := e.open(dir)
	ctx := context.Background()
	snap, _, err := r.StageAll(ctx)
	if err != nil || join(paths(snap.Staged)) != "a.txt,b/c.txt" {
		t.Fatalf("%v %v", paths(snap.Staged), err)
	}
	snap, err = r.UnstageAll(ctx)
	if err != nil || len(snap.Staged) != 0 || join(paths(snap.Untracked)) != "a.txt,b/c.txt" {
		t.Fatalf("%+v %v", snap, err)
	}
	if readFile(t, filepath.Join(dir, "b/c.txt")) != "c\n" {
		t.Fatal("file lost")
	}
	if out := e.git(dir, "rev-list", "--all"); out != "" {
		t.Fatal("committed")
	}
}

func TestStageAllSkipsConflictsAndNestedRepositories(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "c.txt"), "base\n")
	e.commit(dir, "Initial")
	e.git(dir, "switch", "-q", "-c", "other")
	write(t, filepath.Join(dir, "c.txt"), "other\n")
	e.commit(dir, "Other")
	e.git(dir, "switch", "-q", "main")
	write(t, filepath.Join(dir, "c.txt"), "main\n")
	e.commit(dir, "Main")
	e.Runner.Run(context.Background(), dir, nil, "merge", "-q", "other") // conflicts
	write(t, filepath.Join(dir, "plain.txt"), "p\n")
	nested := filepath.Join(dir, "nested")
	write(t, filepath.Join(nested, "x.txt"), "x\n")
	e.git(nested, "init", "-q")
	r, _ := e.open(dir)
	ctx := context.Background()

	snap, res, err := r.StageAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := join(paths(res.Skipped)); got != "c.txt,nested/" {
		t.Fatalf("skipped %s", got)
	}
	find(t, snap.Staged, "plain.txt")
	// The conflict is still unresolved and the nested repository not added.
	if out := e.git(dir, "ls-files", "-u", "--", "c.txt"); strings.Count(out, "\n") != 3 {
		t.Fatalf("conflict stages %q", out)
	}
	if out := e.git(dir, "ls-files", "--stage", "--", "nested"); out != "" {
		t.Fatalf("nested added %q", out)
	}
	// Unstaging everything would drop the conflict: refused.
	_, err = r.UnstageAll(ctx)
	wantCode(t, err, CodeUnstageAllConflicts)
	if out := e.git(dir, "diff", "--cached", "--name-only", "--diff-filter=A"); out != "plain.txt\n" {
		t.Fatalf("index changed: %q", out)
	}
	// Only skipped changes left: nothing to stage.
	_, _, err = r.StageAll(ctx)
	wantCode(t, err, CodeNothingToStage)
}

func TestStageAllFailureShowsRealState(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "1\n")
	r, _ := e.open(dir)
	write(t, filepath.Join(dir, ".git", "index.lock"), "")
	snap, _, err := r.StageAll(context.Background())
	wantCode(t, err, CodeGitFailed)
	if len(snap.Staged) != 0 || len(snap.Untracked) != 1 || snap.Root != dir {
		t.Fatalf("state %+v", snap)
	}
	if readFile(t, filepath.Join(dir, "a.txt")) != "1\n" {
		t.Fatal("file changed")
	}
	// A closed handle refuses.
	e.open(e.repo())
	_, _, err = r.StageAll(context.Background())
	wantCode(t, err, CodeRepositoryClosed)
	_, err = r.UnstageAll(context.Background())
	wantCode(t, err, CodeRepositoryClosed)
}

func TestStageAllLeavesOtherWorktreesAlone(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "1\n")
	e.commit(dir, "Initial")
	linked := filepath.Join(t.TempDir(), "linked")
	e.git(dir, "worktree", "add", "-q", "-b", "side", linked)
	write(t, filepath.Join(dir, "a.txt"), "main\n")
	write(t, filepath.Join(linked, "a.txt"), "side\n")
	write(t, filepath.Join(linked, "b.txt"), "b\n")
	e.git(linked, "add", "b.txt")

	r, _ := e.open(dir)
	ctx := context.Background()
	if _, _, err := r.StageAll(ctx); err != nil {
		t.Fatal(err)
	}
	if out := e.git(linked, "diff", "--cached", "--name-only"); out != "b.txt\n" {
		t.Fatalf("linked index %q", out)
	}
	if _, err := r.UnstageAll(ctx); err != nil {
		t.Fatal(err)
	}
	if out := e.git(linked, "diff", "--cached", "--name-only"); out != "b.txt\n" {
		t.Fatalf("linked index %q", out)
	}
	if readFile(t, filepath.Join(linked, "a.txt")) != "side\n" {
		t.Fatal("linked file changed")
	}
}
