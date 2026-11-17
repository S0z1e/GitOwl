package desktop

import (
	"github.com/egoist/mygo/ui"
	"gitowl/internal/desktop/designicons"

	"gitowl/internal/locales"
)

// kitGallery is the ticket 01 minimal window: custom-drawn controls on the
// design system, driven by the desktop UI tester.
type kitGallery struct {
	segment  int
	name     string
	last     string
	popover  bool
	language string
}

func newKitGallery() *kitGallery {
	return &kitGallery{language: "en"}
}

func (g *kitGallery) t(key string, kv ...any) string {
	var args map[string]any
	if len(kv) > 1 {
		args = make(map[string]any, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			args[kv[i].(string)] = kv[i+1]
		}
	}
	lang := g.language
	if lang == "" {
		lang = "en"
	}
	return locales.Catalog.Format(lang, key, args)
}

// View draws the design-system gallery: primary, secondary and icon
// buttons, a segmented control, a text field, a menu and a popover.
func (g *kitGallery) View(c *ui.Context) {
	t := c.Theme()
	tk := kitDesignTokens(t.Dark)
	ui.Column(c).Fill().Background(tk.bg).Padding(24).Gap(16).Clip().Children(func() {
		ui.Text(c, g.t("kitTitle")).Bold().FontSize(16).TextColor(tk.text)
		g.buttons(c, tk)
		g.toolbar(c, tk)
		g.segments(c, tk)
		g.textField(c, tk)
		g.menuAndPopover(c, tk)
		g.icons(c, tk)
		if g.last != "" {
			ui.Text(c, g.last).TextColor(tk.muted)
		}
	})
}

func (g *kitGallery) buttons(c *ui.Context, tk kitTokens) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Wrap().Children(func() {
		if kitPrimaryButton(c, g.t("kitPrimary"), tk, false).Clicked() {
			g.last = g.t("kitPrimaryClicked")
		}
		kitPrimaryButton(c, g.t("kitDisabledPrimary"), tk, true).Tooltip(g.t("kitUnavailable"))
		if kitSecondaryButton(c, g.t("kitSecondary"), tk, false).Clicked() {
			g.last = g.t("kitSecondaryClicked")
		}
		kitSecondaryButton(c, g.t("kitDisabledSecondary"), tk, true).Tooltip(g.t("kitUnavailable"))
		if kitIconButton(c, designicons.Settings(), g.t("kitIconButton"), tk, false).Clicked() {
			g.last = g.t("kitIconClicked")
		}
		kitIconButton(c, designicons.Settings(), g.t("kitDisabledIcon"), tk, true).Tooltip(g.t("kitUnavailable"))
		ui.Text(c, g.t("kitUnavailable")).FontSize(12).TextColor(tk.muted)
	})
}

func (g *kitGallery) toolbar(c *ui.Context, tk kitTokens) {
	ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
		kitToolbarButton(c, designicons.Fetch(), g.t("fetch"), tk)
		kitToolbarButton(c, designicons.Pull(), g.t("pull"), tk)
		kitToolbarButton(c, designicons.Push(), g.t("push"), tk)
		kitToolbarButton(c, designicons.Stash(), g.t("stash"), tk)
		kitPrimaryButton(c, g.t("kitPrimary"), tk, false)
	})
}

func (g *kitGallery) segments(c *ui.Context, tk kitTokens) {
	unified, split := g.t("kitUnified"), g.t("kitSplit")
	ui.Segmented(c, &g.segment, unified, split)
	option := unified
	if g.segment == 1 {
		option = split
	}
	ui.Text(c, g.t("kitShowing", "option", option)).TextColor(tk.muted)
}

func (g *kitGallery) textField(c *ui.Context, tk kitTokens) {
	ui.Column(c).Gap(6).FillWidth().MaxWidth(280).Children(func() {
		ui.TextInput(c, &g.name).Height(kitFieldH).FillWidth().Label(g.t("kitField"))
		if g.name != "" {
			ui.Text(c, g.name).TextColor(tk.text)
		}
	})
}

func (g *kitGallery) menuAndPopover(c *ui.Context, tk kitTokens) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.MenuButton(c, g.t("kitMenu"), func(m *ui.Menu) {
			if m.Item(g.t("kitMenuAlpha")).Chosen() {
				g.last = g.t("kitChose", "item", g.t("kitMenuAlpha"))
			}
			if m.Item(g.t("kitMenuBeta")).Chosen() {
				g.last = g.t("kitChose", "item", g.t("kitMenuBeta"))
			}
		})
		open := kitSecondaryButton(c, g.t("kitOpenPopover"), tk, false)
		if open.Clicked() {
			g.popover = true
		}
		ui.Popover(c, open, &g.popover, func() {
			ui.Column(c).Gap(8).Padding(8).Children(func() {
				ui.Text(c, g.t("kitPopoverBody")).TextColor(tk.text)
				if kitSecondaryButton(c, g.t("kitClosePopover"), tk, false).Clicked() {
					g.popover = false
				}
			})
		})
	})
}

func (g *kitGallery) icons(c *ui.Context, tk kitTokens) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Wrap().Children(func() {
		ui.Image(c, OwlImage()).Size(kitIconSize, kitIconSize).Label("GitOwl brand")
		for _, ic := range []struct {
			svg   *ui.SVG
			label string
		}{
			{designicons.Sidebar(), g.t("kitSidebar")},
			{designicons.Settings(), g.t("settings")},
			{designicons.Fetch(), g.t("fetch")},
			{designicons.Branch(), g.t("kitBranch")},
			{designicons.Folder(), g.t("kitFolder")},
			{designicons.Changes(), g.t("changesColumn")},
			{designicons.Commits(), g.t("kitAllCommits")},
			{designicons.Editor(), g.t("kitEditor")},
			{designicons.Terminal(), g.t("kitTerminal")},
			{designicons.Finder(), g.t("kitFinder")},
		} {
			ui.Icon(c, ic.svg).Size(kitIconSize, kitIconSize).TextColor(tk.text).Label(ic.label)
		}
	})
}

func kitPrimaryButton(c *ui.Context, label string, tk kitTokens, disabled bool) ui.Element {
	fill, fg := tk.accent, tk.onAc
	if disabled {
		fill, fg = tk.dis, tk.surf
	}
	b := ui.ButtonBase(c).Height(kitPrimaryH).Padding(0, 14).Radius(12).
		Background(fill).TextColor(fg).Label(label).Disabled(disabled)
	b.Children(func() {
		ui.Text(c, label).FontWeight(600).SingleLine()
	})
	return b
}

func kitSecondaryButton(c *ui.Context, label string, tk kitTokens, disabled bool) ui.Element {
	fg := tk.text
	if disabled {
		fg = tk.dis
	}
	b := ui.ButtonBase(c).Height(kitToolbarH).Padding(0, 14).Radius(12).
		Background(tk.surf).TextColor(fg).Border(1, tk.bd2).Label(label).Disabled(disabled)
	b.Children(func() {
		ui.Text(c, label).FontWeight(500).SingleLine()
	})
	return b
}

// kitPillButton is the design's small capsule (.sm): a filled pill without a
// border, for Stage all, Unstage all and a diff card's action.
func kitPillButton(c *ui.Context, label string, tk kitTokens, disabled bool) ui.Element {
	fg := tk.text
	if disabled {
		fg = tk.dis
	}
	b := ui.ButtonBase(c).Height(24).Padding(3, 12).Radius(999).
		Background(tk.fill).TextColor(fg).Label(label).Disabled(disabled)
	b.Children(func() {
		ui.Text(c, label).FontSize(12).FontWeight(500).SingleLine()
	})
	return b
}

// kitPillButtonIcon is kitPillButton with the design's 12-pixel icon before
// the label, as a diff card's Stage and Unstage chunk actions draw it.
func kitPillButtonIcon(c *ui.Context, icon *ui.SVG, label string, tk kitTokens, disabled bool) ui.Element {
	fg := tk.text
	if disabled {
		fg = tk.dis
	}
	b := ui.ButtonBase(c).Height(24).Padding(3, 12).Radius(999).Gap(6).AlignItems(ui.Center).
		Background(tk.fill).TextColor(fg).Label(label).Disabled(disabled)
	b.Children(func() {
		ui.Icon(c, icon).Size(12, 12).TextColor(fg)
		ui.Text(c, label).FontSize(12).FontWeight(500).SingleLine()
	})
	return b
}

// kitSecondaryButtonIcon is a secondary button with an accent icon before its
// label, as the design's Generate Draft is.
func kitSecondaryButtonIcon(c *ui.Context, icon *ui.SVG, label string, tk kitTokens) ui.Element {
	b := ui.ButtonBase(c).Height(kitToolbarH).Padding(0, 14).Gap(6).Radius(12).
		Background(tk.surf).Border(1, tk.bd2).Label(label).AlignItems(ui.Center)
	b.Children(func() {
		ui.Icon(c, icon).Size(14, 14).TextColor(tk.acx)
		ui.Text(c, label).FontWeight(500).TextColor(tk.acx).SingleLine()
	})
	return b
}

func kitToolbarButton(c *ui.Context, icon *ui.SVG, label string, tk kitTokens) ui.Element {
	return kitIconButton(c, icon, label, tk, false)
}

func kitCheckbox(c *ui.Context, on *bool, label string, tk kitTokens, disabled bool) ui.Element {
	// The design's .cx: an 18 box with a 6 radius; unchecked borders use the
	// stronger cxb, and a checked box fills with the accent.
	border, fill := tk.cxb, ui.Color{}
	if *on {
		border, fill = tk.acx, tk.accent
	}
	if disabled {
		border = tk.dis
	}
	// A box, not CheckboxBase: the file list keeps the only Tab stop, and
	// Space on the list stages. The box still names the Stage/Unstage action.
	box := ui.Box(c).Size(18, 18).Label(label).Tooltip(label).Shrink(0)
	if disabled {
		box.Opacity(.45)
	}
	box.Children(func() {
		face := ui.Box(c).Size(18, 18).Radius(6).Border(1, border).Background(fill).Center().PassThrough()
		if *on {
			face.Children(func() {
				ui.Icon(c, designicons.StageCheck()).Size(12, 12).TextColor(tk.onAc)
			})
		}
	})
	return box
}

func kitIconButton(c *ui.Context, icon *ui.SVG, label string, tk kitTokens, disabled bool) ui.Element {
	color := tk.text
	if disabled {
		color = tk.dis
	}
	b := ui.ButtonBase(c).Size(kitToolbarH, kitToolbarH).Radius(12).
		Background(tk.surf).Border(1, tk.bd2).Label(label).Disabled(disabled)
	b.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(color)
	})
	return b
}
