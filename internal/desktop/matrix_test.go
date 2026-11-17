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

// TestLongContentAcrossTheMatrix puts a long branch, a long path, a long
// subject and a long diff line in one repository and checks, at every
// size, appearance and language, that the main actions of Changes, the
// diff and History are on screen and act.
func TestLongContentAcrossTheMatrix(t *testing.T) {
	branch := "feature/" + strings.Repeat("an-extremely-long-branch-name-", 3) + "end"
	file := strings.Repeat("deeply-nested-directory/", 4) + strings.Repeat("name", 10) + ".go"
	subject := "Rework " + strings.Repeat("the very long subject line of a commit ", 4)
	for _, lang := range []string{"en", "zh-Hans"} {
		for _, dark := range []bool{false, true} {
			for _, size := range sizes {
				name := fmt.Sprintf("%s-%dx%d", lang, size[0], size[1])
				if dark {
					name += "-dark"
				}
				t.Run(name, func(t *testing.T) {
					g := gittest.New(t)
					dir := g.Repo()
					gittest.Write(t, filepath.Join(dir, file), "short\n")
					g.Commit(dir, subject)
					g.Git(dir, "switch", "-q", "-c", branch)
					gittest.Write(t, filepath.Join(dir, file), "const x = \""+strings.Repeat("y", 300)+"\"\n")
					p := prefs.Default()
					p.Language = lang
					u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Preferences: p, Go: func(fn func()) { fn() }})
					tt := ui.NewTester(window(u), size[0], size[1])
					tt.SetDark(dark)
					u.OpenPath(dir)
					tt.Frame()
					on := func(s string) ui.Rect {
						t.Helper()
						r, ok := tt.Find(s)
						if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > float32(size[0])+1 || r.Y+r.H > float32(size[1])+1 {
							t.Fatalf("%q at %v (%v) in %v: %q", s, r, ok, size, tt.Texts())
						}
						return r
					}
					click := func(s string) {
						t.Helper()
						r := on(s)
						tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
						tt.Frame()
					}
					// The title bar keeps its actions beside the long branch.
					for _, k := range []string{"settings", "kitEditor", "kitTerminal", "kitFinder"} {
						on(u.t(k))
					}
					on(u.t("commitAction"))
					on(u.t("commitBSummary"))
					// The file's diff and its hunk bar. The taller commit
					// panel B can push the single change row below the fold
					// at the shortest size; scroll it in so the click selects
					// it, keeping the reachability check intact.
					if i, ok := u.wt.row(changeKey{area: workflow.Unstaged, path: file}); ok {
						u.wt.list.ScrollIntoView(i)
						tt.Frame()
					}
					click(filepath.Base(file))
					on(u.t("stageHunk"))
					snapshot(t, tt, "matrix-diff-"+name)
					click(u.t("stageHunk"))
					if got := g.Git(dir, "diff", "--cached", "--name-only"); got != file+"\n" {
						t.Fatalf("index %q", got)
					}
					// History opens on the long subject, with the file.
					if size[0] < narrowWidth {
						click(u.t("chromeBackToList"))
					}
					if size[0] < compactWidth {
						click(sidebarShowLabel(u))
						tt.Advance(time.Second)
					}
					click(u.t("chromeAllCommits"))
					if u.hist.detail == nil || u.hist.detail.Subject != strings.TrimSpace(subject) {
						t.Fatalf("history detail %+v", u.hist.detail)
					}
					snapshot(t, tt, "matrix-history-"+name)
					// The drawer and its long branch, below the wide width.
					if size[0] < compactWidth {
						if _, ok := tt.Find(sidebarShowLabel(u)); ok {
							click(sidebarShowLabel(u))
							tt.Advance(time.Second)
						}
					}
					r := on(u.t("currentBranchName", "branch", branch))
					if r.H > rowHeight+8 {
						t.Fatalf("branch row %v wraps", r)
					}
				})
			}
		}
	}
}
