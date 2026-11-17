package workflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStashIncludesUntracked(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "tracked.txt"), "base\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "tracked.txt"), "edited\n")
	write(t, filepath.Join(dir, "new.txt"), "untracked\n")
	r, _ := e.open(dir)
	ctx := context.Background()

	snap, st, err := r.Stash(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Clean() {
		t.Fatalf("worktree not clean after stash: %+v", snap)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("untracked file still in worktree")
	}
	if st.Ref == "" || st.Hash == "" || st.Message == "" {
		t.Fatalf("stash identity %+v", st)
	}
	list, err := r.Stashes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Ref != st.Ref || list[0].Hash != st.Hash || list[0].Message != st.Message {
		t.Fatalf("stashes %+v, created %+v", list, st)
	}
	if !strings.HasPrefix(st.Ref, "stash@{") {
		t.Fatalf("ref %q", st.Ref)
	}
}

func TestStashRefusesCleanWorktree(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	r, _ := e.open(dir)
	ctx := context.Background()

	snap, st, err := r.Stash(ctx, true)
	wantCode(t, err, CodeNothingToStash)
	if !snap.Clean() || st != (Stash{}) {
		t.Fatalf("empty stash %+v %+v", snap, st)
	}
	list, err := r.Stashes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("created empty stash: %+v", list)
	}
}

func TestStashShowAndDiff(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "tracked.txt"), "base\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "tracked.txt"), "edited\n")
	write(t, filepath.Join(dir, "new.txt"), "untracked\n")
	r, _ := e.open(dir)
	ctx := context.Background()
	_, st, err := r.Stash(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	detail, err := r.StashShow(ctx, st.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Ref != st.Ref || detail.Hash != st.Hash {
		t.Fatalf("detail stash %+v, want %+v", detail.Stash, st)
	}
	if join(paths(detail.Files)) != "tracked.txt,new.txt" && join(paths(detail.Files)) != "new.txt,tracked.txt" {
		t.Fatalf("files %v", paths(detail.Files))
	}

	d, err := r.StashDiff(ctx, st.Ref, "tracked.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Patch, "-base\n") || !strings.Contains(d.Patch, "+edited\n") {
		t.Fatalf("tracked diff %q", d.Patch)
	}
	d, err = r.StashDiff(ctx, st.Ref, "new.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Patch, "+untracked\n") {
		t.Fatalf("untracked diff %q", d.Patch)
	}
}

func TestApplyStashKeepsEntry(t *testing.T) {
	e := newEnv(t)
	r, dir, ref := stashedTracked(t, e, "hello\n")
	ctx := context.Background()

	snap, err := r.ApplyStash(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, "a.txt")) != "hello\n" {
		t.Fatal("apply did not restore the file")
	}
	if join(paths(snap.Unstaged)) != "a.txt" {
		t.Fatalf("after apply %+v", snap)
	}
	list, err := r.Stashes(ctx)
	if err != nil || len(list) != 1 || list[0].Ref != ref {
		t.Fatalf("apply removed stash: %+v %v", list, err)
	}
}

func TestPopStashRemovesEntry(t *testing.T) {
	e := newEnv(t)
	r, dir, ref := stashedTracked(t, e, "popped\n")
	ctx := context.Background()

	snap, err := r.PopStash(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, "a.txt")) != "popped\n" {
		t.Fatal("pop did not restore the file")
	}
	if join(paths(snap.Unstaged)) != "a.txt" {
		t.Fatalf("after pop %+v", snap)
	}
	list, err := r.Stashes(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("pop kept stash: %+v %v", list, err)
	}
}

func TestDropStashRemovesEntry(t *testing.T) {
	e := newEnv(t)
	r, dir, ref := stashedTracked(t, e, "dropped\n")
	ctx := context.Background()
	head := readFile(t, filepath.Join(dir, "a.txt"))

	snap, err := r.DropStash(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, "a.txt")) != head {
		t.Fatal("drop changed the worktree")
	}
	if !snap.Clean() {
		t.Fatalf("after drop %+v", snap)
	}
	list, err := r.Stashes(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("drop kept stash: %+v %v", list, err)
	}
}

func TestRenameStashChangesMessage(t *testing.T) {
	e := newEnv(t)
	r, _, ref := stashedTracked(t, e, "renamed\n")
	ctx := context.Background()
	list, err := r.Stashes(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("before rename %+v %v", list, err)
	}
	hash := list[0].Hash

	snap, err := r.RenameStash(ctx, ref, "save this work")
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Clean() {
		t.Fatalf("rename dirtied worktree %+v", snap)
	}
	list, err = r.Stashes(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("stashes after rename %+v %v", list, err)
	}
	if list[0].Message != "save this work" {
		t.Fatalf("message %q", list[0].Message)
	}
	if list[0].Hash != hash {
		t.Fatalf("rename changed hash %s -> %s", hash, list[0].Hash)
	}
}

func TestBranchFromStashCreatesBranch(t *testing.T) {
	e := newEnv(t)
	r, dir, ref := stashedTracked(t, e, "on-branch\n")
	ctx := context.Background()

	snap, branches, err := r.BranchFromStash(ctx, "from-stash", ref, true)
	if err != nil {
		t.Fatal(err)
	}
	br, ok := branches.local("from-stash")
	if !ok || !br.Current {
		t.Fatalf("missing branch %+v", branches.Local)
	}
	if snap.Branch != "from-stash" {
		t.Fatalf("HEAD branch %q", snap.Branch)
	}
	if readFile(t, filepath.Join(dir, "a.txt")) != "on-branch\n" {
		t.Fatal("stash was not applied on the new branch")
	}
	list, err := r.Stashes(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("stash branch kept the stash: %+v %v", list, err)
	}
}

func TestBranchFromStashWithoutSwitch(t *testing.T) {
	e := newEnv(t)
	r, dir, ref := stashedTracked(t, e, "kept\n")
	ctx := context.Background()
	head := readFile(t, filepath.Join(dir, "a.txt"))

	snap, branches, err := r.BranchFromStash(ctx, "parked", ref, false)
	if err != nil {
		t.Fatal(err)
	}
	br, ok := branches.local("parked")
	if !ok || br.Current {
		t.Fatalf("branch %+v", branches.Local)
	}
	if snap.Branch != "main" {
		t.Fatalf("switched anyway %q", snap.Branch)
	}
	if readFile(t, filepath.Join(dir, "a.txt")) != head {
		t.Fatal("worktree changed without switch")
	}
	list, err := r.Stashes(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("stash dropped without switch: %+v %v", list, err)
	}
}

func TestApplyStashConflictReturnsMarks(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "base\n")
	e.commit(dir, "Base")
	write(t, filepath.Join(dir, "a.txt"), "stashed\n")
	r, _ := e.open(dir)
	ctx := context.Background()
	_, st, err := r.Stash(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "a.txt"), "head\n")
	e.commit(dir, "Moved on")

	snap, err := r.ApplyStash(ctx, st.Ref)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range snap.Unstaged {
		if c.Path == "a.txt" && c.Conflicted {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing conflict mark %+v", snap)
	}
	body := readFile(t, filepath.Join(dir, "a.txt"))
	if !strings.Contains(body, "<<<<<<<") || !strings.Contains(body, "stashed") || !strings.Contains(body, "head") {
		t.Fatalf("worktree %q", body)
	}
	list, err := r.Stashes(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("conflict dropped stash %+v %v", list, err)
	}
}

func stashedTracked(t *testing.T, e *env, edited string) (*Repository, string, string) {
	t.Helper()
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "base\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "a.txt"), edited)
	r, _ := e.open(dir)
	_, st, err := r.Stash(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	return r, dir, st.Ref
}
