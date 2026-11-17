package desktop

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/prefs"
	"gitowl/internal/secrets"
	"gitowl/internal/workflow"
)

// openSettingsAt opens Settings at the given size and lets the dialog
// finish its entrance so measurements are stable.
func openSettingsAt(t *testing.T, u *UI, w, h int) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(window(u), w, h)
	u.OpenSettings()
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	return tt
}

// TestSettingsMeasurements checks the General page against
// reference/measurements/settings.md at the design's 1360×820: the nav
// column (220 wide, its items 192×36), the search box (192×34), the close
// button (34×34 with the design's ✕), the Appearance segmented control and
// the row order (Interface, Language, Appearance, Files, Editor, footer).
// Left edges and control sizes are exact; text widths use near's tolerance.
func TestSettingsMeasurements(t *testing.T) {
	u := New(Options{Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }})
	tt := openSettingsAt(t, u, 1360, 820)

	// The three nav items are 192×36, left-aligned on one column and
	// stacked 36 apart, as the design's .it rows.
	gen, ok := tt.Find("General")
	if !ok {
		t.Fatalf("General not found in %q", tt.Texts())
	}
	for _, s := range []string{"General", "AI Drafts", "About"} {
		r, _ := tt.Find(s)
		if !near(r.W, 192, 1) || !near(r.H, 36, 1) {
			t.Fatalf("nav %q size %v×%v, want 192×36", s, r.W, r.H)
		}
		if !near(r.X, gen.X, 1) {
			t.Fatalf("nav %q x %v, want ~%v", s, r.X, gen.X)
		}
	}
	ai, _ := tt.Find("AI Drafts")
	about, _ := tt.Find("About")
	if !near(ai.Y-gen.Y, 36, 1) || !near(about.Y-ai.Y, 36, 1) {
		t.Fatalf("nav items not 36 apart: %v %v %v", gen.Y, ai.Y, about.Y)
	}

	// The search box: 192 wide, 34 tall, at the nav items' left edge.
	search, ok := tt.Find("Search settings")
	if !ok {
		t.Fatalf("search box not found in %q", tt.Texts())
	}
	if !near(search.W, 192, 1) || !near(search.H, 34, 1) || !near(search.X, gen.X, 1) {
		t.Fatalf("search box %v, want 192×34 at x=%v", search, gen.X)
	}
	if search.Y >= gen.Y {
		t.Fatalf("search box %v below the nav items %v", search.Y, gen.Y)
	}

	// The close button is 34×34 and sits at the content panel's right.
	closeB, ok := tt.Find("Close Settings")
	if !ok {
		t.Fatalf("close button not found in %q", tt.Texts())
	}
	if !near(closeB.W, 34, 1) || !near(closeB.H, 34, 1) {
		t.Fatalf("close button %v, want 34×34", closeB)
	}

	// The content panel starts right of the 220 nav column: the Interface
	// heading and the rows share one left edge, well clear of the nav.
	sec, ok := tt.Find("INTERFACE")
	if !ok {
		t.Fatalf("Interface heading not found (uppercased) in %q", tt.Texts())
	}
	if sec.X-gen.X < 200 {
		t.Fatalf("content left %v too close to nav %v (want a 220 column)", sec.X, gen.X)
	}
	if closeB.X <= sec.X {
		t.Fatalf("close button x %v not at the content's right (sec x %v)", closeB.X, sec.X)
	}

	// Appearance is a segmented control: Light, Dark and System buttons on
	// one baseline, in that order, each 26 tall.
	light, ok1 := tt.Find("Light")
	dark, ok2 := tt.Find("Dark")
	sys, ok3 := tt.Find("System")
	if !ok1 || !ok2 || !ok3 {
		t.Fatalf("segmented control missing a label in %q", tt.Texts())
	}
	if !(light.X < dark.X && dark.X < sys.X) {
		t.Fatalf("segments out of order: Light %v Dark %v System %v", light.X, dark.X, sys.X)
	}
	if !near(light.Y, dark.Y, 1) || !near(dark.Y, sys.Y, 1) {
		t.Fatalf("segments off one baseline: %v %v %v", light.Y, dark.Y, sys.Y)
	}

	// Row order down the page: Interface, Appearance (segments), Files,
	// Editor, footer.
	files, _ := tt.Find("FILES")
	editor, _ := tt.Find("Editor")
	footer, _ := tt.Find("Changes apply immediately.")
	if !(sec.Y < light.Y && light.Y < files.Y && files.Y < editor.Y && editor.Y < footer.Y) {
		t.Fatalf("rows out of order: Interface %v seg %v Files %v Editor %v footer %v",
			sec.Y, light.Y, files.Y, editor.Y, footer.Y)
	}
}

// TestSettingsDialogCentered checks the 800×540 panel is centered in the
// whole 1360×820 window, title bar included: its origin is x=280, y=140
// ((1360−800)/2, (820−540)/2). The panel left edge is the nav column, 14
// inside which the General button sits; the panel top is 20 above the
// close button's row. Both are exact to a couple of DIPs (the header row's
// 22px title adds a sub-pixel to the close button's centered Y).
func TestSettingsDialogCentered(t *testing.T) {
	u := New(Options{Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }})
	tt := openSettingsAt(t, u, 1360, 820)
	gen, ok := tt.Find("General")
	if !ok {
		t.Fatalf("General not found in %q", tt.Texts())
	}
	closeB, ok := tt.Find("Close Settings")
	if !ok {
		t.Fatalf("close button not found in %q", tt.Texts())
	}
	// Panel left edge = the nav column, 14 (nav padding) left of General.
	panelX := gen.X - 14
	if !near(panelX, 280, 2) {
		t.Fatalf("panel x %v (General x %v), want 280", panelX, gen.X)
	}
	// Panel top = 22 (content padding) above the close button's row.
	panelY := closeB.Y - 22
	if !near(panelY, 140, 2) {
		t.Fatalf("panel y %v (close y %v), want 140", panelY, closeB.Y)
	}
	// The close button's right edge = panel right (x=280+800=1080) less the
	// 28 content right padding, confirming the 800 width is centered.
	if !near(closeB.X+closeB.W, 1080-28, 2) {
		t.Fatalf("close right %v, want %v (panel right 1080 − 28)", closeB.X+closeB.W, 1080-28)
	}
}

// TestSettingsAppearanceSegmentApplies switches the appearance with the
// segmented control and checks the choice is saved, applied to the window
// and still in effect when Settings is reopened from the saved file.
func TestSettingsAppearanceSegmentApplies(t *testing.T) {
	var saved []prefs.Preferences
	var applied []prefs.Preferences
	u := New(Options{
		Preferred:        []string{"en-US"},
		Go:               func(fn func()) { fn() },
		SavePreferences:  func(p prefs.Preferences) error { saved = append(saved, p); return nil },
		ApplyPreferences: func(p prefs.Preferences) { applied = append(applied, p) },
	})
	tt := openSettingsAt(t, u, 1360, 820)
	if err := tt.Click("Dark"); err != nil {
		t.Fatalf("click Dark: %v %q", err, tt.Texts())
	}
	tt.Frame()
	if u.prefs.Appearance != "dark" {
		t.Fatalf("appearance not set: %q", u.prefs.Appearance)
	}
	if len(saved) == 0 || saved[len(saved)-1].Appearance != "dark" {
		t.Fatalf("appearance not saved: %+v", saved)
	}
	if len(applied) == 0 || applied[len(applied)-1].Appearance != "dark" {
		t.Fatalf("appearance not applied to the window: %+v", applied)
	}

	// Reopen from the saved preferences: the choice persists, and the
	// language select still names languages in their own language.
	p := saved[len(saved)-1]
	u2 := New(Options{Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }, Preferences: p})
	tt2 := openSettingsAt(t, u2, 1360, 820)
	if u2.prefs.Appearance != "dark" {
		t.Fatalf("reopened appearance %q, want dark", u2.prefs.Appearance)
	}
	if err := tt2.Click("Follow system"); err != nil {
		t.Fatalf("language trigger: %v %q", err, tt2.Texts())
	}
	tt2.Frame()
	if !tt2.HasText("English") || !tt2.HasText("简体中文") {
		t.Fatalf("language options missing: %q", tt2.Texts())
	}
}

// TestSettingsSaveFailureShows reports a failed preference save without
// reverting the choice, which applies for the session.
func TestSettingsSaveFailureShows(t *testing.T) {
	u := New(Options{Preferred: []string{"en-US"}, Go: func(fn func()) { fn() },
		SavePreferences: func(prefs.Preferences) error { return errors.New("disk full") }})
	tt := openSettingsAt(t, u, 1360, 820)
	if err := tt.Click("Dark"); err != nil {
		t.Fatalf("click Dark: %v %q", err, tt.Texts())
	}
	tt.Frame()
	if u.prefs.Appearance != "dark" {
		t.Fatal("save failure reverted the choice")
	}
	if !tt.HasText("Could not save preferences. Your choice applies until GitOwl quits.") || !tt.HasText("disk full") {
		t.Fatalf("no recoverable save-failure feedback: %q", tt.Texts())
	}
}

// TestSettingsKeychainErrorShows surfaces a Keychain failure when saving
// the API key, using the in-memory host double set to fail. The reason is
// shown without echoing the key, and the key is not stored.
func TestSettingsKeychainErrorShows(t *testing.T) {
	keys := &secrets.Memory{Err: errors.New("keychain locked")}
	g := gittest.New(t)
	wf := workflow.New(g.Runner)
	wf.SetAI(workflow.AIConfig{Secrets: keys})
	u := New(Options{Workflow: wf, Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }})
	tt := ui.NewTester(window(u), 1360, 820)
	u.openAISettings()
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	u.form.key = "sk-secret-value"
	tt.Frame()
	if err := tt.Click("Save Key"); err != nil {
		t.Fatalf("Save Key: %v %q", err, tt.Texts())
	}
	tt.Frame()
	if !tt.HasText("keychain locked") {
		t.Fatalf("no Keychain error shown: %q", tt.Texts())
	}
	for _, s := range tt.Texts() {
		if s == "sk-secret-value" {
			t.Fatal("the key value is shown on screen")
		}
	}
	if k, _ := keys.Get(secrets.APIKeyAccount); k == "sk-secret-value" {
		t.Fatal("a failing Keychain still stored the key")
	}
}

// TestSettingsSearchAcrossCategories finds an item of another category by
// a search, and shows a no-results state for a query that matches nothing.
func TestSettingsSearchAcrossCategories(t *testing.T) {
	g := gittest.New(t)
	wf := workflow.New(g.Runner)
	wf.SetAI(workflow.AIConfig{Secrets: &secrets.Memory{}})
	u := New(Options{Workflow: wf, Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }})
	tt := openSettingsAt(t, u, 1360, 820)
	// From General, search reaches an AI Drafts field.
	if err := tt.Click("Search settings"); err != nil {
		t.Fatalf("focus search: %v %q", err, tt.Texts())
	}
	tt.Type("model")
	tt.Frame()
	if !tt.HasText("Model") || !tt.HasText("Results") {
		t.Fatalf("cross-category search missing Model/Results: %q", tt.Texts())
	}
	// Clearing restores a category page; a nonsense query shows no results.
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("zzzznotathing")
	tt.Frame()
	if !tt.HasText("No matching settings") {
		t.Fatalf("no empty-results state: %q", tt.Texts())
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("")
	tt.Key(0, ui.KeyBackspace)
	tt.Frame()
	if !tt.HasText("Language") {
		t.Fatalf("clearing the query did not restore the page: %q", tt.Texts())
	}
}

// TestSettingsSnapshots writes General, AI Drafts and About at 1360×820 in
// both appearances, and a narrow window, to $GITOWL_SNAPSHOT_DIR for review.
func TestSettingsSnapshots(t *testing.T) {
	g := gittest.New(t)
	wf := workflow.New(g.Runner)
	wf.SetAI(workflow.AIConfig{Secrets: &secrets.Memory{}})
	u := New(Options{Workflow: wf, Preferred: []string{"en-US"}, Go: func(fn func()) { fn() },
		Version: "0.1.0"})
	pages := []struct{ id, name string }{
		{"general", "settings-general"},
		{"ai", "settings-ai"},
		{"about", "settings-about"},
	}
	for _, p := range pages {
		u.set.state = settingsState{Category: p.id}
		tt := ui.NewTester(window(u), 1360, 820)
		u.set.open = true
		tt.Frame()
		snapshot(t, tt, p.name+"-light")
		tt.SetDark(true)
		tt.Frame()
		snapshot(t, tt, p.name+"-dark")
	}
	// A narrow window stacks the nav above the content.
	u.set.state = settingsState{Category: "general"}
	tt := ui.NewTester(window(u), 560, 760)
	u.set.open = true
	tt.Frame()
	snapshot(t, tt, "settings-narrow")
}
