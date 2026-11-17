package desktop

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/prefs"
	"gitowl/internal/secrets"
	"gitowl/internal/workflow"
)

func openChromeSettings(t *testing.T, h *harness) {
	t.Helper()
	if err := h.tt.Click("Settings"); err != nil {
		t.Fatalf("gear: %v %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	h.tt.Advance(time.Second)
	h.tt.Frame()
	if !h.u.set.open {
		t.Fatal("gear did not open Settings")
	}
}

func TestSettingsGearOpensDialogWithSearchAndCategories(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	h.tt.Frame()
	openChromeSettings(t, h)
	for _, s := range []string{"Search settings", "General", "AI Drafts", "About", "Language", "Appearance", "Editor"} {
		if !h.tt.HasText(s) {
			t.Fatalf("missing %q in %q", s, h.tt.Texts())
		}
	}
}

func TestSettingsLanguageChangesWelcomeAndKeepsRepositoryNames(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	h.u.prefs.Recent = []string{"/tmp/alpha"}
	h.tt.Frame()
	h.want("alpha", "Pick a repository from the left, or open one from disk to review its changes, history and branches.")

	openChromeSettings(t, h)
	if err := h.tt.Click("Follow system"); err != nil {
		t.Fatalf("language trigger: %v %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	if !h.tt.HasText("English") || !h.tt.HasText("简体中文") {
		t.Fatalf("language names translated: %q", h.tt.Texts())
	}
	if err := h.tt.Click("简体中文"); err != nil {
		t.Fatalf("choose language: %v %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	if h.u.prefs.Language != "zh-Hans" {
		t.Fatalf("language %q", h.u.prefs.Language)
	}

	if err := h.tt.Click("关闭设置"); err != nil {
		t.Fatalf("close: %v %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	h.tt.Advance(time.Second)
	h.tt.Frame()
	if h.u.set.open {
		t.Fatal("settings stayed open")
	}
	h.want("从左侧选择仓库，或从磁盘打开，\n以审阅修改、历史和分支。", "尚未打开仓库", "alpha")
	if h.tt.HasText("Pick a repository from the left, or open one from disk to review its changes, history and branches.") {
		t.Fatalf("welcome stayed in English: %q", h.tt.Texts())
	}
}

func TestSettingsLanguageChangesOpenedRepositorySentences(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "hello\n")
	name := filepath.Base(dir)
	h.u.prefs.Recent = []string{dir}
	h.tt.Frame()
	h.click(name)
	h.want("a.txt", name)

	openChromeSettings(t, h)
	if err := h.tt.Click("Follow system"); err != nil {
		t.Fatalf("language trigger: %v %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	if err := h.tt.Click("简体中文"); err != nil {
		t.Fatalf("choose language: %v %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	if err := h.tt.Click("关闭设置"); err != nil {
		t.Fatalf("close: %v %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	h.tt.Advance(time.Second)
	h.tt.Frame()
	h.want("a.txt", name, "未跟踪")
}

func TestSettingsEscapeReturnsFocusToGear(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	h.tt.Frame()
	h.tt.SetFocused(true)
	openChromeSettings(t, h)
	h.tt.Key(0, ui.KeyEscape)
	h.tt.Frame()
	h.tt.Advance(time.Second)
	h.tt.Frame()
	if h.u.set.open {
		t.Fatal("Escape left Settings open")
	}
	if !h.tt.Focused("Settings") {
		t.Fatalf("focus on %q after Escape, want Settings", focusedLabels(h.tt))
	}
}

func TestSettingsNarrowUsesCategorySelect(t *testing.T) {
	h, _ := chromeHarness(t, 560, 760)
	h.tt.Frame()
	openChromeSettings(t, h)
	h.want("Category", "General", "Language", "Appearance", "Editor")
	if err := h.tt.Click("Category"); err != nil {
		t.Fatalf("category: %v %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	if err := h.tt.Click("AI Drafts"); err != nil {
		t.Fatalf("choose AI Drafts: %v %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	for _, s := range []string{"Service URL", "Model", "API key", "Model context", "Commit message prompt"} {
		if !h.tt.HasText(s) {
			t.Fatalf("AI Drafts missing %q in %q", s, h.tt.Texts())
		}
	}
}

func TestSettingsSaveFailureKeepsChoiceOnChrome(t *testing.T) {
	g := gittest.New(t)
	u := New(Options{
		Workflow:        workflow.New(g.Runner),
		Preferred:       []string{"en-US"},
		Go:              func(fn func()) { fn() },
		SavePreferences: func(prefs.Preferences) error { return errors.New("disk full") },
	})
	tt := ui.NewTester(chromeWindow(u), 1280, 800)
	tt.Frame()
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("Dark"); err != nil {
		t.Fatalf("Dark: %v %q", err, tt.Texts())
	}
	tt.Frame()
	if u.prefs.Appearance != "dark" {
		t.Fatal("save failure reverted the choice")
	}
	if !tt.HasText("Could not save preferences. Your choice applies until GitOwl quits.") || !tt.HasText("disk full") {
		t.Fatalf("no save-failure feedback: %q", tt.Texts())
	}
}

func TestSettingsAIDraftsAndAboutKeepExistingCopy(t *testing.T) {
	g := gittest.New(t)
	wf := workflow.New(g.Runner)
	wf.SetAI(workflow.AIConfig{Secrets: &secrets.Memory{}})
	u := New(Options{Workflow: wf, Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }, Version: "0.1.0"})
	tt := ui.NewTester(chromeWindow(u), 1280, 800)
	tt.Frame()
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("About"); err != nil {
		t.Fatalf("About: %v %q", err, tt.Texts())
	}
	tt.Frame()
	for _, s := range []string{
		"GitOwl",
		"Version 0.1.0",
		"A lightweight macOS Git client for reviewing changes and history, managing local branches and preparing commit messages.",
	} {
		if !tt.HasText(s) {
			t.Fatalf("About missing %q in %q", s, tt.Texts())
		}
	}
	if err := tt.Click("AI Drafts"); err != nil {
		t.Fatalf("AI Drafts: %v %q", err, tt.Texts())
	}
	tt.Frame()
	for _, s := range []string{
		"Service URL",
		"An OpenAI-compatible API base. GitOwl posts to <URL>/chat/completions. Use https, or http only for a service on this Mac.",
		"API key",
		"Commit message prompt",
	} {
		if !tt.HasText(s) {
			t.Fatalf("AI Drafts missing existing copy %q in %q", s, tt.Texts())
		}
	}
}

func focusedLabels(tt *ui.Tester) []string {
	var f []string
	for _, s := range tt.Texts() {
		if tt.Focused(s) {
			f = append(f, s)
		}
	}
	return f
}
