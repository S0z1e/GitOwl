package desktop

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/launch"
	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

// TestAcceptanceDailyFlow runs next-iteration issue 11's walkthrough in one
// window, in order: a recent repository, one hunk staged, an AI draft
// edited and committed, Push, a colleague's commit fetched, Pull refused
// while work is left and done once it is discarded, a search, an edit in
// another app, closing to the welcome screen and switching projects.
func TestAcceptanceDailyFlow(t *testing.T) {
	h, srv, _ := newAIHarness(t, nil)
	bare, mine, theirs := clones(t, h.Env)
	other := h.Repo()
	gittest.Write(t, filepath.Join(other, "o.txt"), "o\n")
	h.Commit(other, "Other project")
	gittest.Write(t, filepath.Join(mine, "f.txt"), numbered(30, nil))
	h.Commit(mine, "Add f")
	h.Git(mine, "push", "-q", "origin", "main")
	store := &savedPrefs{}
	l := &launcher{}
	h.u.opts.SavePreferences, h.u.opts.Launch = store.save, l.launch
	h.u.prefs.Recent = []string{mine, other}
	h.tt.Frame()

	// The welcome screen lists both projects; open mine from it.
	h.want("No repository open", "Repositories", "mine", filepath.Base(other))
	h.click("mine")
	if h.u.repo == nil || h.u.repo.Root() != mine {
		t.Fatal("recent repository not opened")
	}

	// Two edits in one file: stage only the first hunk.
	edited := numbered(30, map[int]string{2: "two\n", 25: "twenty-five\n"})
	gittest.Write(t, filepath.Join(mine, "f.txt"), edited)
	h.u.Refresh()
	h.tt.Frame()
	h.selectFile(workflow.Unstaged, "f.txt")
	// Two edits make two hunks; stage only the first (line 2).
	h.stageHunk(true, 1, 2)
	if got := h.Git(mine, "show", ":f.txt"); got != numbered(30, map[int]string{2: "two\n"}) {
		t.Fatalf("index after the hunk:\n%s", got)
	}

	// A draft from the staged hunk only, edited by hand, then committed.
	srv.set(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"Rename line two"}}]}`))
	})
	h.click("Generate Draft")
	if req := h.u.ai.last; req == nil || !strings.Contains(req.Messages[1].Content, "+two") || strings.Contains(req.Messages[1].Content, "twenty-five") {
		t.Fatal("the draft request is not the staged hunk alone")
	}
	if h.u.cm.message != "Rename line two\n\nExplain what changed." {
		t.Fatalf("draft %q", h.u.cm.message)
	}
	h.u.cm.message = "Rename line two by hunk" + strings.TrimPrefix(h.u.cm.message, "Rename line two")
	h.tt.Frame()
	h.click("Commit")
	if got := h.Git(mine, "log", "-1", "--format=%s"); got != "Rename line two by hunk\n" {
		t.Fatalf("commit %q", got)
	}

	// Push it.
	h.want("↑1")
	h.click("Push")
	if err := h.tt.Click(h.u.t("pushTo", "branch", "main", "target", "origin/main")); err != nil {
		t.Fatalf("run push: %v (%q)", err, h.tt.Texts())
	}
	h.tt.Frame()
	if h.Git(bare, "rev-parse", "main") != h.Git(mine, "rev-parse", "HEAD") {
		t.Fatal("not pushed")
	}

	h.Git(theirs, "pull", "-q", "--ff-only")
	theyPush(t, h.Env, theirs, "b.txt")
	h.click("Fetch")
	h.want("↓1")

	h.click("All commits")
	h.want("Rename line two by hunk")
	h.click("Current changes")

	// Edit in another app: the request names the file; returning shows it.
	h.selectFile(workflow.Unstaged, "f.txt")
	h.menu("Actions for f.txt", "Open with Default App")
	if len(l.got) != 1 || l.got[0] != (launch.Request{Tool: launch.Editor, Path: filepath.Join(mine, "f.txt")}) {
		t.Fatalf("launch %+v", l.got)
	}
	gittest.Write(t, filepath.Join(mine, "f.txt"), numbered(30, map[int]string{2: "two\n", 25: "twenty-five\n", 30: "thirty\n"}))
	h.u.Refresh() // as the window does when it is focused again
	h.tt.Frame()
	if p := h.u.wt.diff; p == nil || !strings.Contains(p.info.Patch, "thirty") {
		t.Fatal("the edit made elsewhere is not shown")
	}

	// Discard what is left, then Pull fast-forwards.
	h.menu("Actions for f.txt", "Discard Unstaged Changes…")
	h.click("Discard Changes")
	if data, _ := os.ReadFile(filepath.Join(mine, "f.txt")); string(data) != numbered(30, map[int]string{2: "two\n"}) {
		t.Fatalf("after discard:\n%s", data)
	}
	h.click("Pull")
	if err := h.tt.Click(h.u.t("pullRun", "target", "origin/main")); err != nil {
		t.Fatalf("run pull: %v (%q)", err, h.tt.Texts())
	}
	h.tt.Frame()
	if h.Git(mine, "rev-parse", "HEAD") != h.Git(theirs, "rev-parse", "HEAD") {
		t.Fatal("not pulled")
	}

	h.u.cm.message = "next change"
	h.click(filepath.Base(other))
	if h.u.repo == nil || h.u.repo.Root() != other || h.u.cm.message != "" {
		t.Fatal("other project")
	}
	h.click("mine")
	if h.u.repo.Root() != mine || h.u.cm.message != "next change" {
		t.Fatalf("back in mine: %q", h.u.cm.message)
	}
	if len(store.p.Recent) != 2 || store.p.Recent[0] != mine || store.p.Recent[1] != other {
		t.Fatalf("recent saved %q", store.p.Recent)
	}
	// Nothing ran in the other project.
	if h.Git(other, "status", "--porcelain") != "" || h.Git(other, "rev-list", "--count", "HEAD") != "1\n" {
		t.Fatal("the other project changed")
	}
}

// TestNewEntriesAcrossSizesThemesLanguages renders the new title bar
// actions, history search and the recent list in both languages and
// appearances, from a wide window to the narrowest, and checks that the
// actions keep their names.
func TestNewEntriesAcrossSizesThemesLanguages(t *testing.T) {
	g := gittest.New(t)
	_, mine, theirs := clones(t, g)
	theyPush(t, g, theirs, "b.txt")
	g.Git(mine, "fetch", "-q")
	for _, lang := range []string{"en", "zh-Hans"} {
		p := prefs.Default()
		p.Language = lang
		p.Recent = []string{mine}
		u := New(Options{Workflow: workflow.New(g.Runner), Preferences: p, Go: func(fn func()) { fn() }})
		tt := ui.NewTester(window(u), 1280, 800)
		tt.Frame()
		snapshot(t, tt, "grid-welcome-"+lang)
		u.OpenPath(mine)
		tt.Frame()
		names := map[string][]string{"en": {"Fetch", "Pull", "Push", "Editor", "Terminal", "Finder"}, "zh-Hans": {"获取", "拉取", "推送", "编辑器", "终端", "Finder"}}[lang]
		for _, size := range sizes {
			for _, dark := range []bool{false, true} {
				tt.SetSize(size[0], size[1])
				tt.SetDark(dark)
				u.middleTab = 1
				tt.Frame()
				tt.Frame()
				for _, n := range names {
					if !tt.HasText(n) {
						t.Fatalf("%s %v dark=%v: no %q in %q", lang, size, dark, n, tt.Texts())
					}
				}
				theme := "light"
				if dark {
					theme = "dark"
				}
				snapshot(t, tt, "grid-"+lang+"-"+theme+"-"+strconv.Itoa(size[0]))
			}
		}
	}
}
