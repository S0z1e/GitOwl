package desktop

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

// sizes are the window sizes every state is checked at: wide, the drawer,
// the narrow list and detail, and the minimum window.
var sizes = [][2]int{{1280, 800}, {900, 700}, {600, 600}, {560, 480}}

// TestFourStatesAcrossSizesThemesAndLanguages walks welcome, clean,
// changes and history at every size, in both appearances and languages,
// and checks the main action of each state is on screen.
func TestFourStatesAcrossSizesThemesAndLanguages(t *testing.T) {
	type texts struct{ open, clean, history, commit, message, stage, commitTitle, back string }
	langs := map[string]texts{
		"en":      {"Open Repository…", "Current changes", "All commits", "Commit", "Summary", "Stage a.txt", "Initial", "Back"},
		"zh-Hans": {"打开仓库…", "当前更改", "全部提交", "提交", "摘要", "暂存 a.txt", "Initial", "返回"},
	}
	for _, lang := range []string{"en", "zh-Hans"} {
		want := langs[lang]
		for _, dark := range []bool{false, true} {
			for _, size := range sizes {
				name := fmt.Sprintf("%s-%dx%d", lang, size[0], size[1])
				if dark {
					name += "-dark"
				}
				t.Run(name, func(t *testing.T) {
					g := gittest.New(t)
					dir := g.Repo()
					gittest.Write(t, filepath.Join(dir, "base.txt"), "base\n")
					g.Commit(dir, "Initial")
					p := prefs.Default()
					p.Language = lang
					u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Preferences: p, Go: func(fn func()) { fn() }})
					tt := ui.NewTester(window(u), size[0], size[1])
					tt.SetDark(dark)
					tt.Frame()
					find := func(state, s string) {
						t.Helper()
						if _, ok := tt.Find(s); !ok {
							t.Fatalf("%s: %q not on screen in %q", state, s, tt.Texts())
						}
					}
					click := func(s string) {
						t.Helper()
						if err := tt.Click(s); err != nil {
							t.Fatalf("click %q: %v in %q", s, err, tt.Texts())
						}
						tt.Frame()
					}
					find("welcome", want.open)
					snapshot(t, tt, "state-welcome-"+name)

					u.OpenPath(dir)
					tt.Frame()
					if size[0] < compactWidth {
						click(sidebarShowLabel(u))
						tt.Advance(time.Second)
					}
					find("clean", want.clean)
					find("clean", want.history)
					snapshot(t, tt, "state-clean-"+name)
					if size[0] < compactWidth {
						click(sidebarHideLabel(u))
						tt.Advance(time.Second)
					}

					gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
					u.Refresh()
					tt.Frame()
					find("changes", want.stage)
					find("changes", want.message)
					find("changes", want.commit)
					// The taller panel B can push the only file below the
					// fold at the shortest size; scroll it in so the stage
					// click lands, keeping the reachability check intact.
					if i, ok := u.wt.row(changeKey{area: workflow.Untracked, path: "a.txt"}); ok {
						u.wt.list.ScrollIntoView(i)
						tt.Frame()
					}
					click(want.stage)
					if got := g.Git(dir, "diff", "--cached", "--name-only"); got != "a.txt\n" {
						t.Fatalf("index %q", got)
					}
					snapshot(t, tt, "state-changes-"+name)

					// The diff of a file, with a way back when narrow.
					i, _ := u.wt.row(changeKey{area: workflow.Staged, path: "a.txt"})
					u.selectRow(i)
					tt.Frame()
					if u.wt.diff == nil || u.wt.diff.state == nil {
						t.Fatal("no diff")
					}
					if size[0] < narrowWidth {
						click(want.back)
					}
					u.cm.message = "Add a"
					tt.Frame()
					click(want.commit)
					if got := g.Git(dir, "log", "-1", "--format=%s"); got != "Add a\n" {
						t.Fatalf("commit %q", got)
					}

					// History: list, then the commit with its files and diff.
					if size[0] < compactWidth {
						click(sidebarShowLabel(u))
						tt.Advance(time.Second)
					}
					find("clean after commit", want.history)
					click(want.history)
					find("history", "Add a")
					u.selectCommit(0)
					tt.Frame()
					if u.hist.detail == nil {
						t.Fatal("no history detail")
					}
					find("history detail", "a.txt")
					snapshot(t, tt, "state-history-"+name)
				})
			}
		}
	}
}

func TestLongNamesKeepActionsReachable(t *testing.T) {
	long := strings.Repeat("very-long-directory-name/", 6)
	for _, size := range sizes {
		t.Run(fmt.Sprint(size[0]), func(t *testing.T) {
			h := newHarness(t, size[0], size[1])
			dir := h.Repo()
			gittest.Write(t, filepath.Join(dir, "seed.txt"), "s\n")
			h.Commit(dir, "Initial")
			branch := "feature/" + strings.Repeat("an-extremely-long-branch-name-", 3)
			h.Git(dir, "switch", "-q", "-c", branch)
			// The same file name in several directories, and a long one.
			for _, d := range []string{"a", "b", "c"} {
				gittest.Write(t, filepath.Join(dir, d, "index.ts"), d+"\n")
			}
			file := long + strings.Repeat("name", 12) + ".go"
			gittest.Write(t, filepath.Join(dir, file), "x\n")
			h.open(dir)
			// Rows below the fold are scrolled to, as a user would.
			stage := func(path string) {
				t.Helper()
				i, _ := h.u.wt.row(changeKey{area: workflow.Untracked, path: path})
				h.u.wt.list.ScrollIntoView(i)
				h.tt.Frame()
				h.click("Stage " + path)
			}
			for _, s := range []string{"Commit", "Summary", "Stage a/index.ts"} {
				if _, ok := h.tt.Find(s); !ok {
					t.Fatalf("%q not on screen in %q", s, h.tt.Texts())
				}
			}
			stage(file)
			stage("b/index.ts")
			if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "b/index.ts\n"+file+"\n" {
				t.Fatalf("index %q", got)
			}
			// The repository name and branch stay in the title bar.
			h.want(filepath.Base(dir), branch)
			snapshot(t, h.tt, "long-names-"+itoa(size[0]))
		})
	}
}
