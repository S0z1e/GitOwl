package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// kitWindow is the ticket 01 seam: a desktop UI tester over the custom-draw
// gallery, with the design-system theme and no Onevra shell.
func kitWindow(g *kitGallery) func(*ui.Context) {
	return func(c *ui.Context) {
		c.SetTheme(KitTheme(c))
		g.View(c)
	}
}

func TestKitGalleryOpensOnDesignSystem(t *testing.T) {
	g := newKitGallery()
	tt := ui.NewTester(kitWindow(g), 800, 600)
	for _, want := range []string{"Design system", "Primary", "Secondary"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	for _, s := range []string{"Working tree clean", "Open a repository to review its changes."} {
		if tt.HasText(s) {
			t.Fatalf("gallery shows terracotta shell %q in %q", s, tt.Texts())
		}
	}
	snapshot(t, tt, "kit-gallery-light")
	tt.SetDark(true)
	if !tt.HasText("Design system") || !tt.HasText("Primary") {
		t.Fatalf("dark gallery lost the design-system surface: %q", tt.Texts())
	}
	snapshot(t, tt, "kit-gallery-dark")
}

func TestKitButtonsReportEnabledAndDisabled(t *testing.T) {
	g := newKitGallery()
	tt := ui.NewTester(kitWindow(g), 800, 600)
	if err := tt.Click("Primary"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Primary clicked") {
		t.Fatalf("enabled primary did nothing: %q", tt.Texts())
	}
	if err := tt.Click("Disabled Primary"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Primary clicked") && g.last != "Primary clicked" {
		t.Fatal("disabled primary changed the last action")
	}
	if g.last != "Primary clicked" {
		t.Fatalf("disabled primary ran: %q", g.last)
	}
	if !tt.HasText("Unavailable in this gallery") {
		t.Fatalf("disabled reason missing in %q", tt.Texts())
	}

	if err := tt.Click("Secondary"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if g.last != "Secondary clicked" {
		t.Fatalf("enabled secondary: %q", g.last)
	}
	if err := tt.Click("Disabled Secondary"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if g.last != "Secondary clicked" {
		t.Fatalf("disabled secondary ran: %q", g.last)
	}

	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if g.last != "Settings clicked" {
		t.Fatalf("enabled icon: %q", g.last)
	}
	if err := tt.Click("Disabled Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if g.last != "Settings clicked" {
		t.Fatalf("disabled icon ran: %q", g.last)
	}
}

func TestKitSegmentedSwitchesVisibleOption(t *testing.T) {
	g := newKitGallery()
	tt := ui.NewTester(kitWindow(g), 800, 600)
	if !tt.HasText("Showing Unified") {
		t.Fatalf("default segment: %q", tt.Texts())
	}
	if err := tt.Click("Split"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Showing Split") || tt.HasText("Showing Unified") {
		t.Fatalf("after Split: %q", tt.Texts())
	}
	if err := tt.Click("Unified"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Showing Unified") {
		t.Fatalf("after Unified: %q", tt.Texts())
	}
}

func TestKitTextFieldAcceptsInput(t *testing.T) {
	g := newKitGallery()
	tt := ui.NewTester(kitWindow(g), 800, 600)
	if err := tt.Click("Name"); err != nil {
		t.Fatal(err)
	}
	tt.Type("hello")
	tt.Frame()
	if g.name != "hello" {
		t.Fatalf("field %q", g.name)
	}
	if !tt.HasText("hello") {
		t.Fatalf("typed text missing in %q", tt.Texts())
	}
}

func TestKitMenuOpensAndChooses(t *testing.T) {
	g := newKitGallery()
	tt := ui.NewTester(kitWindow(g), 800, 600)
	if err := tt.Click("Actions"); err != nil {
		t.Fatalf("open menu: %v (texts %q)", err, tt.Texts())
	}
	if got := tt.Menu(); len(got) == 0 {
		t.Fatalf("menu did not open, items %q", got)
	}
	if err := tt.ChooseMenuItem("Rename"); err != nil {
		t.Fatalf("choose: %v (items %q)", err, tt.Menu())
	}
	tt.Frame()
	if !tt.HasText("Chose Rename") {
		t.Fatalf("after menu: %q", tt.Texts())
	}
}

func TestKitPopoverOpensAndCloses(t *testing.T) {
	g := newKitGallery()
	tt := ui.NewTester(kitWindow(g), 800, 600)
	if tt.HasText("Close popover") {
		t.Fatal("popover open at start")
	}
	if err := tt.Click("Open popover"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Popover") || !tt.HasText("Close popover") {
		t.Fatalf("popover did not open: %q", tt.Texts())
	}
	if err := tt.Click("Close popover"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Close popover") {
		t.Fatalf("popover stayed open: %q", tt.Texts())
	}
}

func TestKitToolbarButtonsShareHeightAndPrimaryIsTaller(t *testing.T) {
	g := newKitGallery()
	tt := ui.NewTester(kitWindow(g), 800, 600)
	var h float32
	for _, label := range []string{"Fetch", "Pull", "Push", "Stash"} {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("missing toolbar %q in %q", label, tt.Texts())
		}
		if h == 0 {
			h = r.H
		}
		if r.H != h {
			t.Fatalf("%s height %v, want shared %v", label, r.H, h)
		}
	}
	primary, ok := tt.Find("Primary")
	if !ok {
		t.Fatal("primary missing")
	}
	if primary.H <= h {
		t.Fatalf("primary height %v, want taller than toolbar %v", primary.H, h)
	}
}

func TestKitIconsAreSVGNotLucide(t *testing.T) {
	g := newKitGallery()
	tt := ui.NewTester(kitWindow(g), 800, 600)
	for _, label := range []string{"GitOwl brand", "Sidebar", "Settings", "Fetch", "Branch", "Folder", "Changes", "All commits", "Editor", "Terminal", "Finder"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("missing SVG icon %q in %q", label, tt.Texts())
		}
	}
}
