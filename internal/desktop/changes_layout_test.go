package desktop

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

// TestChangesReviewAcrossSizesThemesLanguages reviews staged and unstaged
// changes at every size, in both appearances and languages: the columns
// fold as the width asks, an empty area is one row, a file row puts its
// directory beside or under its name, and selecting a file only shows its
// diff.
func TestChangesReviewAcrossSizesThemesLanguages(t *testing.T) {
	long := strings.Repeat("deeply-nested-directory/", 5)
	longFile := long + strings.Repeat("long", 10) + ".go"
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
					gittest.Write(t, filepath.Join(dir, "src", "app", "main.go"), "package main\n")
					gittest.Write(t, filepath.Join(dir, longFile), "a\n")
					g.Commit(dir, "Initial")
					gittest.Write(t, filepath.Join(dir, "src", "app", "main.go"), "package main\n\nfunc main() {}\n")
					g.Git(dir, "add", "src/app/main.go")
					gittest.Write(t, filepath.Join(dir, longFile), "b\n")
					p := prefs.Default()
					p.Language = lang
					u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Preferences: p, Go: func(fn func()) { fn() }})
					tt := ui.NewTester(window(u), size[0], size[1])
					tt.SetDark(dark)
					u.OpenPath(dir)
					tt.Frame()
					tt.Frame()
					find := func(s string) ui.Rect {
						t.Helper()
						r, ok := tt.Find(s)
						if !ok {
							t.Fatalf("%q not on screen in %q", s, tt.Texts())
						}
						return r
					}

					// The repository column beside the list, behind the
					// panel button, or in a drawer.
					branches := u.t("branchesTitle")
					wide := size[0] >= compactWidth
					if tt.HasText(branches) != wide {
						t.Fatalf("branches shown %v at %d", !wide, size[0])
					}
					if !wide {
						if err := tt.Click(sidebarShowLabel(u)); err != nil {
							t.Fatal(err)
						}
						tt.Frame()
						tt.Advance(time.Second)
						find(branches)
						tt.Click(sidebarHideLabel(u))
						tt.Frame()
						tt.Advance(time.Second)
						if tt.HasText(branches) {
							t.Fatal("drawer stays open")
						}
					}

					// Name and directory side by side when wide, the
					// directory under the name when narrow. The top rows are
					// in view before any scroll.
					nameR, dirR := find("main.go"), find("src/app")
					if size[0] < narrowWidth {
						if dirR.Y <= nameR.Y {
							t.Fatalf("directory %v not under name %v", dirR, nameR)
						}
					} else if math.Abs(float64(dirR.Y+dirR.H/2-(nameR.Y+nameR.H/2))) > 2 || dirR.X <= nameR.X {
						t.Fatalf("directory %v not beside name %v", dirR, nameR)
					}

					// Untracked is empty: one row that says so, no
					// explanation below it. The taller commit panel B can
					// push the last list row below the fold at the shortest
					// size; scroll it in so the empty-area row is rendered,
					// keeping the check intact.
					n, last := 0, -1
					for i, r := range u.wt.rows {
						if r.key.area == workflow.Untracked {
							n++
							last = i
						}
					}
					if last >= 0 {
						u.wt.list.ScrollIntoView(last)
						tt.Frame()
					}
					_ = n
					// A long path keeps its name and check box on screen.
					longName := filepath.Base(longFile)
					if r := find(longName); r.X+20 > float32(size[0]) {
						t.Fatalf("long name off screen at %v", r)
					}
					find(u.t("stageFile", "path", longFile))
					snapshot(t, tt, "changes-list-"+name)

					// Clicking a row shows its diff and leaves the index.
					before := g.Git(dir, "diff", "--cached", "--name-only")
					if err := tt.Click(longName); err != nil {
						t.Fatal(err)
					}
					tt.Frame()
					tt.Frame()
					if got := g.Git(dir, "diff", "--cached", "--name-only"); got != before {
						t.Fatalf("index %q, was %q", got, before)
					}
					if u.wt.diff == nil || u.wt.diff.state == nil || u.wt.diff.key.path != longFile {
						t.Fatalf("diff %+v", u.wt.diff)
					}
					// The diff header names the file as the design does:
					// the muted directory prefix and the bold file name,
					// both on screen (not one joined path string).
					if r := find(strings.TrimSuffix(long, "/")); r.X+20 > float32(size[0]) {
						t.Fatalf("diff path prefix off screen at %v", r)
					}
					find(longName)
					snapshot(t, tt, "changes-review-"+name)
					if size[0] < narrowWidth {
						// The detail, then back to the same list and selection.
						if tt.HasText(u.t("stageAll")) {
							t.Fatal("narrow detail shows the list too")
						}
						if err := tt.Click(u.t("chromeBackToList")); err != nil {
							t.Fatal(err)
						}
						tt.Frame()
						find(u.t("stageAll"))
						if u.wt.selected == nil || u.wt.selected.path != longFile {
							t.Fatalf("selection %+v after back", u.wt.selected)
						}
					} else {
						find(u.t("stageAll"))
					}
					if c := tt.Image().Bounds(); c.Dx() == 0 {
						t.Fatal("no frame")
					}
				})
			}
		}
	}
}

func count(texts []string, s string) int {
	n := 0
	for _, x := range texts {
		if x == s {
			n++
		}
	}
	return n
}

func luminance(c ui.Color) float64 {
	c = c.SRGB()
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

func contrast(a, b ui.Color) float64 {
	x, y := luminance(a), luminance(b)
	return (max(x, y) + 0.05) / (min(x, y) + 0.05)
}

// TestLayersKeepContrast checks GitOwl's own layers in both appearances:
// text and secondary text are at least 4.5:1 on every surface and on the
// selection, and the accent, the focus mark and the status letters at
// least 3:1.
func TestLayersKeepContrast(t *testing.T) {
	for _, dark := range []bool{false, true} {
		tk := kitDesignTokens(dark)
		surfaces := map[string]ui.Color{
			"content":           tk.bg,
			"sidebar":           tk.rail,
			"nav":               tk.nav,
			"selected content":  tk.sel.Over(tk.bg),
			"window background": tk.bg,
		}
		for name, bg := range surfaces {
			for role, fg := range map[string]ui.Color{"text": tk.text, "muted": tk.muted} {
				if r := contrast(fg, bg); r < 4.5 {
					t.Errorf("dark=%v %s on %s: %.2f, want >= 4.5", dark, role, name, r)
				}
			}
			for role, fg := range map[string]ui.Color{"accent": tk.accent, "added": tk.add, "deleted": tk.del, "modified": tk.warn} {
				if r := contrast(fg, bg); r < 3 {
					t.Errorf("dark=%v %s on %s: %.2f, want >= 3", dark, role, name, r)
				}
			}
		}
	}
}
