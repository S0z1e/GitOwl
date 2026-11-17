package desktop

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

func TestLanguageResolution(t *testing.T) {
	for _, tc := range []struct {
		choice string
		system []string
		want   string
	}{
		{"system", []string{"zh-CN"}, "zh-Hans"},
		{"system", []string{"zh-Hans-CN"}, "zh-Hans"},
		{"system", []string{"en-GB"}, "en"},
		{"system", []string{"de-DE"}, "en"},
		{"system", []string{"de-DE", "zh-CN"}, "zh-Hans"},
		{"system", nil, "en"},
		{"en", []string{"zh-CN"}, "en"},
		{"zh-Hans", []string{"en-US"}, "zh-Hans"},
	} {
		if got := ResolveLanguage(tc.choice, tc.system); got != tc.want {
			t.Errorf("%s %v: %s, want %s", tc.choice, tc.system, got, tc.want)
		}
	}
}

func TestSettingsChangeLanguageAndAppearance(t *testing.T) {
	var saved, applied []prefs.Preferences
	u := New(Options{
		Preferred:        []string{"en-US"},
		SavePreferences:  func(p prefs.Preferences) error { saved = append(saved, p); return nil },
		ApplyPreferences: func(p prefs.Preferences) { applied = append(applied, p) },
	})
	tt := ui.NewTester(window(u), 1280, 800)
	u.OpenSettings()
	tt.Frame()
	for _, s := range []string{"Language", "Appearance", "Follow system"} {
		if !tt.HasText(s) {
			t.Fatalf("missing %q in %q", s, tt.Texts())
		}
	}
	snapshot(t, tt, "settings")
	// The first "Follow system" is the language select's trigger.
	if err := tt.Click("Follow system"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("简体中文"); err != nil {
		t.Fatalf("choose language: %v %q", err, tt.Texts())
	}
	tt.Frame()
	if u.prefs.Language != "zh-Hans" || len(saved) != 1 || saved[0].Language != "zh-Hans" || len(applied) != 1 {
		t.Fatalf("language not saved: %+v %+v", u.prefs, saved)
	}
	// Appearance is a segmented control; its System segment reads as the
	// short "系统" (issue 18), not the select's longer "跟随系统".
	for _, s := range []string{"语言", "外观", "系统"} {
		if !tt.HasText(s) {
			t.Fatalf("missing %q after switching to Chinese: %q", s, tt.Texts())
		}
	}
	// The language list names languages in their own language.
	if u.optionText("en") != "English" {
		t.Fatal(u.optionText("en"))
	}
	u.setPreferences(prefs.Preferences{Language: "zh-Hans", Appearance: "dark"})
	if last := applied[len(applied)-1]; last.Appearance != "dark" {
		t.Fatalf("appearance not applied: %+v", last)
	}
	snapshot(t, tt, "settings-zh")
}

func TestPreferenceSaveFailureKeepsChoice(t *testing.T) {
	u := New(Options{Preferred: []string{"en-US"}, SavePreferences: func(prefs.Preferences) error { return errors.New("disk full") }})
	tt := ui.NewTester(window(u), 1280, 800)
	u.OpenSettings()
	u.setPreferences(prefs.Preferences{Language: "en", Appearance: "light"})
	tt.Frame()
	if u.prefs.Appearance != "light" {
		t.Fatal("choice reverted")
	}
	if !tt.HasText("Could not save preferences. Your choice applies until GitOwl quits.") || !tt.HasText("disk full") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestInterfaceTextFollowsLanguage(t *testing.T) {
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() },
		Preferences: prefs.Preferences{Language: "zh-Hans", Appearance: "system"}})
	tt := ui.NewTester(window(u), 1280, 800)
	tt.Frame()
	for _, s := range []string{"GitOwl", "尚未打开仓库", "打开仓库…"} {
		if !tt.HasText(s) {
			t.Fatalf("missing %q in %q", s, tt.Texts())
		}
	}
	u.OpenPath(dir)
	tt.Frame()
	// Product name, branch and file names stay as they are.
	for _, s := range []string{"main", "a.txt", "未跟踪", "未跟踪文件", "提交"} {
		if !tt.HasText(s) {
			t.Fatalf("missing %q in %q", s, tt.Texts())
		}
	}
	snapshot(t, tt, "opened-zh")
}

func TestLayoutsKeepWorkflowsReachable(t *testing.T) {
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	g.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "b\n")
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }})
	for _, dark := range []bool{false, true} {
		for _, size := range [][2]int{{1280, 800}, {900, 700}, {600, 600}} {
			tt := ui.NewTester(window(u), size[0], size[1])
			tt.SetDark(dark)
			u.OpenPath(dir)
			u.lay = layout{}
			tt.Frame()
			// Staging and committing are reachable at every width.
			for _, s := range []string{"a.txt", "Commit", "Summary"} {
				if _, ok := tt.Find(s); !ok && !tt.HasText(s) {
					t.Fatalf("%v dark=%v: missing %q in %q", size, dark, s, tt.Texts())
				}
			}
			if err := tt.Click("Stage a.txt"); err != nil {
				t.Fatalf("%v: stage: %v", size, err)
			}
			tt.Frame()
			if err := tt.Click("Unstage a.txt"); err != nil {
				t.Fatalf("%v: unstage: %v", size, err)
			}
			tt.Frame()
			// The repository column: inline when wide, behind the toggle otherwise.
			wide := size[0] >= compactWidth
			if tt.HasText("Branches") != wide {
				t.Fatalf("%v: branches inline=%v", size, tt.HasText("Branches"))
			}
			if !wide {
				if err := tt.Click(sidebarShowLabel(u)); err != nil {
					t.Fatalf("%v: %v", size, err)
				}
				tt.Advance(time.Second) // the drawer slides in before its rows show
				tt.Frame()
				if !tt.HasText("Branches") {
					t.Fatalf("%v: drawer missing branches %q", size, tt.Texts())
				}
				snapshot(t, tt, "drawer-"+itoa(size[0]))
				u.lay.drawerOpen = false
				tt.Frame()
			}
			// Narrow windows show the diff after a selection, with a way back.
			i, _ := u.wt.row(changeKey{area: workflow.Untracked, path: "a.txt"})
			u.selectRow(i)
			tt.Frame()
			narrow := size[0] < narrowWidth
			if narrow {
				if err := tt.Click("a.txt"); err == nil {
					tt.Frame()
				}
				if _, ok := tt.Find("Back"); ok {
					if err := tt.Click("Back"); err != nil {
						t.Fatalf("%v: back: %v", size, err)
					}
					tt.Frame()
				}
				if !tt.HasText("Commit") {
					t.Fatalf("%v: missing Commit in %q", size, tt.Texts())
				}
			}
			name := "layout-" + itoa(size[0])
			if dark {
				name += "-dark"
			}
			snapshot(t, tt, name)
		}
	}
}

func itoa(n int) string {
	return string(rune('0'+n/1000%10)) + string(rune('0'+n/100%10)) + string(rune('0'+n/10%10)) + string(rune('0'+n%10))
}
