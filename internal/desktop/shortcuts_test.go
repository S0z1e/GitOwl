package desktop

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// key presses a shortcut and lets the repeat window pass.
func (h *harness) key(mods ui.Modifiers, k ui.Key) {
	h.T.Helper()
	h.tt.Key(mods, k)
	h.tt.Frame()
	h.u.lastActionAt = time.Time{}
}

func TestShortcutsRespectBusyState(t *testing.T) {
	g := gittest.New(t)
	a, _ := twoRepos(t, g)
	q := &queued{}
	chose := 0
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go,
		ChooseDirectory: func(string) (string, error) { chose++; return "", nil }})
	tt := ui.NewTester(window(u), 1280, 800)
	u.OpenPath(a)
	q.drain()
	tt.Frame()
	u.bulkStage(false)
	for _, a := range []Action{ActionOpen, ActionRecent, ActionClose, ActionWorktrees, ActionFetch, ActionPull, ActionPush} {
		if u.Can(a) {
			t.Errorf("%s allowed during Stage All", a)
		}
	}
	u.lastActionAt = time.Time{}
	tt.Key(ui.Cmd, ui.KeyO)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyW)
	tt.Frame()
	if chose != 0 || u.repo == nil || u.pick.open {
		t.Fatal("a shortcut got around Stage All")
	}
	// Refresh only reads: it stays available.
	if !u.Can(ActionRefresh) {
		t.Fatal("refresh blocked")
	}
	q.drain()
	tt.Frame()
	if !u.Can(ActionClose) || !u.Can(ActionOpen) {
		t.Fatal("actions still blocked afterwards")
	}
}

func TestOneKeyPressRunsOnce(t *testing.T) {
	g := gittest.New(t)
	chose := 0
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go,
		ChooseDirectory: func(string) (string, error) { chose++; return "", nil }})
	// The menu's key equivalent and the window both see ⌘O.
	u.Do(ActionOpen)
	u.Do(ActionOpen)
	q.drain()
	if chose != 1 {
		t.Fatalf("picker shown %d times", chose)
	}
}

func TestTypingKeepsShortcutsAway(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "b\n")
	h.open(dir)
	h.typeMessage("Fix: rollout")
	if h.u.cm.message != "Fix: rollout" || h.u.pick.open {
		t.Fatalf("message %q", h.u.cm.message)
	}
	h.key(ui.Cmd, ui.KeyComma)
	if !h.u.set.open {
		t.Fatal("⌘, did not open Settings")
	}
}
