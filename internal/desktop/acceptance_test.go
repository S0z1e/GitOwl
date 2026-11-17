package desktop

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitowl/internal/gittest"
	"gitowl/internal/secrets"
	"gitowl/internal/workflow"
)

// TestAcceptanceWalkthrough runs issue 09's checklist in one window, in
// order, as a tester would: first commit with leftover changes, history and
// bases, branches, an AI draft through a local service, failures that keep
// the message, and protection against stale results.
func TestAcceptanceWalkthrough(t *testing.T) {
	h, srv, _ := newAIHarness(t, nil)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "README.md"), "# Demo\n")
	gittest.Write(t, filepath.Join(dir, "main.go"), "package main\n")
	gittest.Write(t, filepath.Join(dir, "notes.txt"), "not for this commit\n")

	// Open, review, stage and unstage, then the first commit.
	h.open(dir)
	h.want("main", "No commits yet", "Untracked files")
	h.selectFile(workflow.Untracked, "README.md")
	h.want("+1", "−0")
	h.click("Stage README.md")
	h.click("Stage main.go")
	h.click("Unstage main.go")
	h.click("Stage main.go")
	h.typeMessage("Add README and main")
	h.click("Commit")
	h.want("Untracked files", "notes.txt")
	if got := h.Git(dir, "show", "--name-only", "--format=", "HEAD"); got != "README.md\nmain.go\n" {
		t.Fatalf("first commit files %q", got)
	}

	// History: the root commit compares with the empty tree.
	h.click("All commits")
	h.want("Add README and main")
	h.u.selectCommit(0)
	h.tt.Frame()
	h.want("Add README and main")
	h.click("Current changes")

	// Branches: create, commit on it, merge, and the first parent base.
	h.chooseBranchMenu(h.u.t("currentBranchName", "branch", "main"), "New branch from here")
	h.u.br.form.name = "feature"
	h.tt.Frame()
	h.click("Create Branch")
	h.tt.Frame()
	h.branchAction("feature", "Checkout")
	gittest.Write(t, filepath.Join(dir, "feature.go"), "package main\n// feature\n")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Stage feature.go")

	// An AI draft: a failure that keeps words, retry, edit, commit.
	h.u.cm.message = "draft by hand"
	srv.set(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":{"message":"overloaded"}}`))
	})
	h.tt.Frame()
	h.click("Generate Draft")
	if strings.Contains(h.u.ai.last.Messages[1].Content, "not for this commit") {
		t.Fatal("untracked content in the request")
	}
	h.want("The service reported an error. Your message is kept.")
	if h.u.cm.message != "draft by hand" {
		t.Fatal("failure lost the message")
	}
	srv.set(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"Add feature"}}]}`))
	})
	h.click("Generate Draft")
	h.click("Replace Message")
	if out := h.Git(dir, "rev-list", "--count", "HEAD"); out != "1\n" {
		t.Fatal("a draft committed")
	}
	h.u.cm.message += "\n\nReviewed."
	h.tt.Frame()
	h.click("Commit")
	if got := h.Git(dir, "log", "-1", "--format=%B"); got != "Add feature\n\nExplain what changed.\n\nReviewed.\n\n" {
		t.Fatalf("commit message %q", got)
	}
	if h.u.snap.Branch != "feature" {
		t.Fatal(h.u.snap.Branch)
	}

	// Back to main: history no longer shows feature's commit.
	h.branchAction("main", "Checkout")
	// History reads again at the new HEAD. Its scope stays All branches, so
	// feature's commits can still be listed.
	if h.u.hist.branch != h.u.snap.Branch {
		t.Fatalf("history read at %q, the checkout is %q", h.u.hist.branch, h.u.snap.Branch)
	}
	// A merge made outside GitOwl compares with its first parent.
	h.Git(dir, "merge", "-q", "--no-ff", "-m", "Merge feature", "feature")
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	h.u.selectCommit(0)
	h.tt.Frame()
	h.want("Merge feature", "Parents", h.u.hist.detail.BaseHash[:7])
	h.click("Current changes")
	// The merged branch is deleted after confirmation.
	h.branchAction("feature", "Delete branch…")
	h.click("Delete")
	if strings.Contains(h.Git(dir, "branch"), "feature") {
		t.Fatal("merged branch not deleted")
	}

	// A switch that would overwrite local work fails and keeps it.
	h.Git(dir, "branch", "old", "HEAD~1")
	gittest.Write(t, filepath.Join(dir, "feature.go"), "precious local work\n")
	h.u.Refresh()
	h.tt.Frame()
	h.branchAction("old", "Checkout")
	h.want("Git did not switch branches because local changes would be overwritten. Your changes are kept; commit them or move them aside, then switch again.")
	if data, _ := os.ReadFile(filepath.Join(dir, "feature.go")); string(data) != "precious local work\n" {
		t.Fatalf("local work lost: %q", data)
	}

	// Staging changes while a draft is written marks it stale; nothing is
	// committed and the message is not replaced.
	h.click("Stage feature.go")
	h.u.cm.message = ""
	srv.set(func(w http.ResponseWriter, r *http.Request) {
		h.Git(dir, "add", "notes.txt")
		w.Write([]byte(`{"choices":[{"message":{"content":"Update feature"}}]}`))
	})
	h.click("Generate Draft")
	h.want("Use Anyway")
	if h.u.cm.message != "" || h.Git(dir, "log", "-1", "--format=%s") != "Merge feature\n" {
		t.Fatal("stale draft applied or committed")
	}
	snapshot(t, h.tt, "acceptance")
}

func TestAcceptanceAcrossWorktrees(t *testing.T) {
	q := &queued{}
	h, srv, keys := newAIHarness(t, q)
	keys.Delete(secrets.APIKeyAccount) // a local service without a key
	main, linked := twoWorktrees(t, h.Env)
	gittest.Write(t, filepath.Join(main, "a.txt"), "main work\n")
	gittest.Write(t, filepath.Join(linked, "b.txt"), "linked work\n")
	run := func() { q.drain(); h.tt.Frame() }
	h.u.OpenPath(main)
	run()

	// A: Stage All, a hand-written message.
	h.click("Stage All")
	run()
	h.u.cm.message = "Main by hand"
	h.tt.Frame()

	// B: Stage All; a draft is asked for, then the user goes back to A
	// before it answers.
	h.u.OpenPath(linked)
	run()
	h.click("Stage All")
	run()
	h.click("Generate Draft")
	h.u.OpenPath(main)
	q.runLast()
	run()
	if h.u.repo.Root() != main || h.u.cm.message != "Main by hand" || h.u.ai.pending != nil {
		t.Fatalf("B's draft reached A: %q %+v", h.u.cm.message, h.u.ai.pending)
	}
	// Back in B: the old request does not fill its new opening.
	h.u.OpenPath(linked)
	run()
	if h.u.cm.message != "" || h.u.ai.pending != nil || len(h.u.snap.Staged) != 1 {
		t.Fatalf("B on return: %q %d staged", h.u.cm.message, len(h.u.snap.Staged))
	}
	// A fresh draft suggests for B's empty message; two requests each time.
	calls := srv.calls()
	h.click("Generate Draft")
	run()
	if srv.calls() != calls+2 {
		t.Fatalf("B draft calls %d", srv.calls()-calls)
	}
	if h.u.cm.message != draftText {
		t.Fatalf("B fill %q", h.u.cm.message)
	}
	// Staging changes while a draft is written: marked stale, not applied.
	h.u.cm.message = "Edited in B"
	h.tt.Frame()
	h.click("Generate Draft")
	gittest.Write(t, filepath.Join(linked, "c.txt"), "c\n")
	h.Git(linked, "add", "c.txt")
	run()
	if h.u.cm.message != "Edited in B" || h.u.ai.pending == nil || !h.u.ai.pending.Stale {
		t.Fatalf("stale: %q %+v", h.u.cm.message, h.u.ai.pending)
	}
	// Ticket 06: a stale-vs-staging card is Use Anyway / Regenerate, not
	// Onevra's Discard. The handwritten message stays; Commit uses it.
	h.want("Use Anyway", "Regenerate")
	if h.tt.HasText("Replace Message") {
		t.Fatal("stale card kept Replace Message")
	}
	h.u.Refresh()
	run()
	h.click("Commit")
	run()
	if got := h.Git(linked, "log", "-1", "--format=%s"); got != "Edited in B\n" {
		t.Fatalf("B commit %q", got)
	}
	// A's index and message are untouched by all of it.
	h.u.OpenPath(main)
	run()
	if h.u.cm.message != "Main by hand" || h.Git(main, "diff", "--cached", "--name-only") != "a.txt\n" {
		t.Fatalf("A: %q", h.u.cm.message)
	}
	h.click("Commit")
	run()
	if got := h.Git(main, "log", "-1", "--format=%s"); got != "Main by hand\n" || h.Git(linked, "status", "--porcelain") != "" {
		t.Fatalf("A commit %q", got)
	}
	snapshot(t, h.tt, "acceptance-worktrees")
}
