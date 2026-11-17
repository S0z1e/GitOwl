package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTwoWorktreesEndToEnd runs the whole workflow in two worktrees of one
// repository: create, Stage All, a draft from a local service without a
// key (one shortened request), edit, commit, switch back and forth, then
// remove and prune. Each worktree's index, files, HEAD and branch stay its
// own throughout.
func TestTwoWorktreesEndToEnd(t *testing.T) {
	e, f, keys, cfg := aiEnv(t)
	keys.Delete("ai-api-key")
	main := e.repo()
	write(t, filepath.Join(main, "app.go"), bigSource("App", 1500, "v1"))
	e.commit(main, "Add the app")
	ctx := context.Background()

	a, _ := e.open(main)
	path := SuggestWorktreePath(main, "feature/big")
	_, _, linked, err := a.CreateWorktree(ctx, NewWorktree{Branch: "feature/big", Create: true, Path: path})
	if err != nil {
		t.Fatal(err)
	}

	// Work in both before staging anything.
	write(t, filepath.Join(main, "main.txt"), "main work\n")
	write(t, filepath.Join(linked, "app.go"), bigSource("App", 1500, "v2")) // a large change
	write(t, filepath.Join(linked, "package-lock.json"), strings.Repeat("{\"a\": 1},\n", 2000))

	// B: Stage All, one shortened draft, an edited commit.
	b, _ := e.open(linked)
	snap, _, err := b.StageAll(ctx)
	if err != nil || len(snap.Staged) != 2 {
		t.Fatalf("stage all in B: %v %d", err, len(snap.Staged))
	}
	if e.git(main, "diff", "--cached", "--name-only") != "" || readFile(t, filepath.Join(main, "app.go")) != bigSource("App", 1500, "v1") {
		t.Fatal("B's staging touched A")
	}
	req, d, err := b.Draft(ctx, DraftOptions{}, cfg)
	if err != nil || f.count() != 2 || f.last().Auth != "" || !req.Shortened() || req.Estimate > req.Budget.Input {
		t.Fatalf("draft in B: %v calls %d auth %q shortened %v", err, f.count(), f.last().Auth, req.Shortened())
	}
	if strings.Contains(f.last().Messages[1].Content, "main work") {
		t.Fatal("A's file in B's request")
	}
	// Viewing a request sends nothing.
	if _, err := b.PrepareDraft(ctx, DraftOptions{}); err != nil || f.count() != 2 {
		t.Fatal("view sent")
	}
	if _, _, err := b.Commit(ctx, d.Text+"\n\nEdited.", snap.StagedFingerprint); err != nil {
		t.Fatal(err)
	}
	if e.git(linked, "log", "-1", "--format=%B") != draftText+"\n\nEdited.\n\n" || e.git(linked, "branch", "--show-current") != "feature/big\n" {
		t.Fatal("B's commit")
	}

	// A: its own staging and commit, on main.
	a, _ = e.open(main)
	snap, _, err = a.StageAll(ctx)
	if err != nil || len(snap.Staged) != 1 || snap.Branch != "main" {
		t.Fatalf("stage all in A: %v %+v", err, snap.Staged)
	}
	if _, _, err := a.Commit(ctx, "Add main work", snap.StagedFingerprint); err != nil {
		t.Fatal(err)
	}
	if e.git(main, "log", "-1", "--format=%s") != "Add main work\n" || strings.Contains(e.git(main, "log", "--format=%s"), "Add greeting") {
		t.Fatal("A's history mixed with B's")
	}
	if e.git(linked, "status", "--porcelain") != "" {
		t.Fatal("A's commit touched B")
	}
	// Unstage All in A leaves B alone.
	write(t, filepath.Join(main, "more.txt"), "more\n")
	write(t, filepath.Join(linked, "kept.txt"), "kept\n")
	e.git(linked, "add", "kept.txt")
	if _, _, err := a.StageAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := a.UnstageAll(ctx); err != nil {
		t.Fatal(err)
	}
	if e.git(linked, "diff", "--cached", "--name-only") != "kept.txt\n" {
		t.Fatal("A's Unstage All touched B")
	}

	// A late draft for B, after A→B→A, is reported closed.
	b, _ = e.open(linked)
	breq, err := b.PrepareDraft(ctx, DraftOptions{})
	if err != nil {
		t.Fatal(err)
	}
	e.open(main)
	e.open(linked)
	a, _ = e.open(main)
	_, err = b.GenerateDraft(ctx, breq, cfg)
	wantCode(t, err, CodeRepositoryClosed)
	if f.count() != 2 {
		t.Fatal("a closed opening sent a request")
	}

	// B is not clean: removal refused; once clean, removed, branch kept.
	_, err = a.RemoveWorktree(ctx, linked)
	wantCode(t, err, CodeWorktreeDirty)
	e.git(linked, "reset", "-q")
	os.Remove(filepath.Join(linked, "kept.txt"))
	if _, err := a.RemoveWorktree(ctx, linked); err != nil {
		t.Fatal(err)
	}
	if e.git(main, "branch", "--list", "feature/big") == "" {
		t.Fatal("branch deleted")
	}
	// A third worktree removed outside GitOwl is pruned as a record only.
	third := addWorktree(t, e, main, "third")
	os.RemoveAll(third)
	pruned, list, err := a.Prune(ctx)
	if err != nil || len(pruned) != 1 || len(list) != 1 || !list[0].Main {
		t.Fatalf("prune %+v %+v %v", pruned, list, err)
	}
	if readFile(t, filepath.Join(main, "more.txt")) != "more\n" {
		t.Fatal("main's file changed")
	}
}
