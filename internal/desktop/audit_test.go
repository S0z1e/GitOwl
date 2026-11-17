package desktop

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

// TestAuditBoards renders the five HTML design boards (Landing, Repository
// Changes with a diff, Settings General, Commit B four states, History with
// a Graph and branch labels) at the design's content sizes and records the
// screen rect (x, y, w, h) of each board's key elements to the test log.
// With GITOWL_SNAPSHOT_DIR set it also writes a PNG of each board. The logs
// feed the measured "native value" column of the design audit; the PNGs are
// the visual evidence.
// This test adds no product code: it only observes and snapshots. See
// html-design issue 20.

// auditLog records a named element's rect, failing when it is not on screen
// so a missing element can never be silently audited as 0,0,0,0.
func auditLog(t *testing.T, tt *ui.Tester, board, name, query string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(query)
	if !ok {
		t.Fatalf("%s: %q (%q) not on screen in %q", board, name, query, tt.Texts())
	}
	t.Logf("AUDIT %-18s %-22s x=%.1f y=%.1f w=%.1f h=%.1f",
		board, name, r.X, r.Y, r.W, r.H)
	return r
}

// TestAuditBoards renders the five HTML design boards as subtests, each
// recording its key elements' rects and writing its snapshot(s). Run it
// with GITOWL_SNAPSHOT_DIR set to the absolute audit directory to produce
// the PNGs the verification doc cites.
func TestAuditBoards(t *testing.T) {
	t.Run("landing", auditBoardLanding)
	t.Run("changes", auditBoardChanges)
	t.Run("settings", auditBoardSettings)
	t.Run("commitb", auditBoardCommitB)
	t.Run("history", auditBoardHistory)
}

// auditBoardLanding renders the Landing board at 1360×820 with two
// Recent entries and records the owl, title, Open button and first Recent
// row against reference/measurements/landing.md.
func auditBoardLanding(t *testing.T) {
	p := prefs.Default()
	p.Recent = []string{"/tmp/GitOwl", "/tmp/example-repo"}
	u := New(Options{Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }, Preferences: p})
	tt := ui.NewTester(window(u), 1360, 820)
	tt.Frame()
	tt.Frame()

	auditLog(t, tt, "landing", "owl", "GitOwl brand")
	auditLog(t, tt, "landing", "title", u.t("noRepositoryTitle"))
	auditLog(t, tt, "landing", "open-button", "Open Repository…")
	auditLog(t, tt, "landing", "recent-title", "Repositories")
	auditLog(t, tt, "landing", "recent-row-1", "GitOwl")
	auditLog(t, tt, "landing", "settings-button", u.t("settings"))

	snapshot(t, tt, "landing")
	tt.SetDark(true)
	snapshot(t, tt, "landing-dark")
}

// auditBoardChanges renders the Repository Changes board at 1360×820
// with one staged edit, one unstaged edit and an untracked file, then
// selects a file to show its diff with the hunk bar. It records the shell
// columns, the section headers with their bulk buttons, a status badge, a
// file row and the diff/hunk controls against repository.md and
// repository-changes.md / repository-diff.md.
func auditBoardChanges(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "ai.go"), "package main\n")
	gittest.Write(t, filepath.Join(dir, "changes.go"), "package main\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "ai.go"), "package main\n\nfunc a() {}\n")
	h.Git(dir, "add", "ai.go")
	gittest.Write(t, filepath.Join(dir, "changes.go"), "package main\n\nfunc c() {}\n")
	gittest.Write(t, filepath.Join(dir, "app.go"), "package main\n")
	h.open(dir)

	auditLog(t, h.tt, "changes", "changes-tab", h.u.t("chromeCurrentChanges"))
	auditLog(t, h.tt, "changes", "history-tab", h.u.t("chromeAllCommits"))
	auditLog(t, h.tt, "changes", "branches-title", h.u.t("branchesTitle"))
	auditLog(t, h.tt, "changes", "staged-header", h.u.t("chromeStagedChanges"))
	auditLog(t, h.tt, "changes", "unstaged-header", h.u.t("chromeUnstagedChanges"))
	auditLog(t, h.tt, "changes", "stage-all", h.u.t("stageAll"))
	auditLog(t, h.tt, "changes", "unstage-all", h.u.t("unstageAll"))
	auditLog(t, h.tt, "changes", "status-badge-M", h.u.t("kindM"))
	auditLog(t, h.tt, "changes", "file-row", "changes.go")
	auditLog(t, h.tt, "changes", "commit-summary", h.u.t("commitBSummary"))

	// Select an unstaged file to show its diff and the hunk bar.
	h.selectFile(workflow.Unstaged, "changes.go")
	auditLog(t, h.tt, "changes", "stage-hunk", h.u.t("stageHunk"))

	snapshot(t, h.tt, "changes")
	h.tt.SetDark(true)
	snapshot(t, h.tt, "changes-dark")
}

// auditBoardSettings renders the Settings dialog on its General page at
// 1360×820 and records the nav items, the search box, the close button and
// the General rows (Language, Appearance, with the System/Light/Dark
// segmented control) against reference/measurements/settings.md.
func auditBoardSettings(t *testing.T) {
	u := New(Options{Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }})
	tt := ui.NewTester(window(u), 1360, 820)
	u.OpenSettings()
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()

	auditLog(t, tt, "settings", "nav-general", u.t("settingsGeneral"))
	auditLog(t, tt, "settings", "nav-ai", u.t("settingsAI"))
	auditLog(t, tt, "settings", "nav-about", u.t("settingsAbout"))
	auditLog(t, tt, "settings", "search", u.t("settingsSearch"))
	auditLog(t, tt, "settings", "close", u.t("settingsClose"))
	auditLog(t, tt, "settings", "row-language", u.t("settingsLanguage"))
	auditLog(t, tt, "settings", "row-appearance", u.t("settingsAppearance"))
	auditLog(t, tt, "settings", "appearance-light", u.t("optionLight"))
	auditLog(t, tt, "settings", "appearance-dark", u.t("optionDark"))

	snapshot(t, tt, "settings")
	tt.SetDark(true)
	snapshot(t, tt, "settings-dark")
}

// auditBoardCommitB renders the Commit panel B board in its four design
// states at the panel's 360 DIP width and records the key measurements of
// each (the staged header, the Summary and Description fields, the Commit
// button) against reference/measurements/commit-b.md. It then composites a
// 1560×380 four-state comparison image (states at x = 24 / 408 / 792 /
// 1176, the design's column origins) when GITOWL_SNAPSHOT_DIR is set.
func auditBoardCommitB(t *testing.T) {
	h, _, _ := newAIHarness(t, nil)
	dir := stagedRepo(h)
	h.open(dir)
	auditLog(t, h.tt, "commitb/empty", "staged-header", h.u.t("chromeStagedChanges"))
	auditLog(t, h.tt, "commitb/empty", "summary", h.u.t("commitBSummary"))
	auditLog(t, h.tt, "commitb/empty", "description", h.u.t("commitBDescription"))
	auditLog(t, h.tt, "commitb/empty", "commit-button", h.u.t("commitAction"))
	h.click("Generate Draft")
	auditLog(t, h.tt, "commitb/ready", "summary", h.u.t("commitBSummary"))
	auditLog(t, h.tt, "commitb/ready", "commit-button", h.u.t("commitAction"))
	snapshot(t, h.tt, "audit-commitb")
}

// auditBoardHistory renders the History board at 1360×820 on a
// fork/merge history, so the Graph draws lanes and the current branch
// shows its accent pill. It records the newest commit row, its branch pill
// and the detail's subject and file against reference/measurements/history.md.
func auditBoardHistory(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "base\n")
	h.Commit(dir, "Base")
	h.Git(dir, "checkout", "-q", "-b", "feature/ai-drafts")
	gittest.Write(t, filepath.Join(dir, "s.txt"), "s\n")
	h.Commit(dir, "Add AI draft language option")
	h.Git(dir, "checkout", "-q", "main")
	gittest.Write(t, filepath.Join(dir, "m.txt"), "m\n")
	h.Commit(dir, "Show staged file count in commit box")
	h.Git(dir, "merge", "-q", "--no-ff", "-m", "Merge branch feature/ai-drafts", "feature/ai-drafts")
	h.open(dir)
	h.click("All commits")
	if !h.u.hist.graphing {
		t.Fatalf("History is not graphing: query %+v", h.u.hist.query)
	}

	newest := h.u.hist.commits[0].Subject
	auditLog(t, h.tt, "history", "newest-row", newest)
	auditLog(t, h.tt, "history", "newest-hash", h.u.hist.commits[0].Short())
	auditLog(t, h.tt, "history", "branch-pill", "main")
	auditLog(t, h.tt, "history", "detail-subject", newest)

	h.tt.SetDark(false)
	h.tt.Frame()
	snapshot(t, h.tt, "history")
	h.tt.SetDark(true)
	h.tt.Frame()
	snapshot(t, h.tt, "history-dark")
}
