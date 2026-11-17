package desktop

import (
	"gitowl/internal/desktop/designicons"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/prefs"
)

// settingsState is the selected Settings page and search query.
type settingsState struct {
	Category string
	Query    string
}

type settingsCategory struct {
	ID, Title, Description string
	Groups                 []settingsGroup
}

type settingsGroup struct {
	ID, Title string
	Items     []settingsItem
}

type settingsItem struct {
	ID, Title, Description string
	Keywords               []string
	Build                  func(c *ui.Context)
	Stacked                bool
	Content                func(c *ui.Context)
}

type settingsResult struct{ Category, Group, Item int }

// settingsPanel is the state of the Settings dialog.
type settingsPanel struct {
	open  bool
	state settingsState
	// saveErr is the last failure to save preferences; the choice still
	// applies for this session.
	saveErr string
}

// Preferences returns the current preferences.
func (u *UI) Preferences() prefs.Preferences { return u.prefs }

func (u *UI) prefLanguage() string { return u.prefs.Language }

// OpenSettings shows the Settings dialog.
func (u *UI) OpenSettings() { u.set.open = true }

// setPreferences saves p and applies it to the app.
func (u *UI) setPreferences(p prefs.Preferences) {
	p.Normalize()
	if p.Equal(u.prefs) {
		return
	}
	before := u.prefs
	u.prefs = p
	u.set.saveErr = ""
	if u.opts.SavePreferences != nil {
		if err := u.opts.SavePreferences(p); err != nil {
			u.set.saveErr = err.Error()
		}
	}
	// AI settings change as they are typed; only the interface needs applying.
	if u.opts.ApplyPreferences != nil && (p.Language != before.Language || p.Appearance != before.Appearance) {
		u.opts.ApplyPreferences(p)
	}
	u.titleChanged()
}

// optionText names a setting value. Language names are written in their
// own language and are not translated.
func (u *UI) optionText(value string) string {
	switch value {
	case "en":
		return "English"
	case "zh-Hans":
		return "简体中文"
	case prefs.System:
		return u.t("optionSystem")
	case "light":
		return u.t("optionLight")
	case "dark":
		return u.t("optionDark")
	case "on":
		return u.t("optionOn")
	case "off":
		return u.t("optionOff")
	}
	return value
}

func (u *UI) choice(c *ui.Context, label string, value string, choices []string, set func(string)) {
	u.kitChoice(c, label, value, choices, u.optionText, set)
}

// kitChoice is a labeled select drawn with kit surfaces: a 36-tall field
// that opens a popover of the choices. Visible names stay on the row;
// the control's accessible name is label.
func (u *UI) kitChoice(c *ui.Context, label, value string, choices []string, text func(string) string, set func(string)) {
	tk := kitDesignTokens(c.Theme().Dark)
	trigger := ui.ButtonBase(c).Key("choice:"+label).Label(label).Height(34).
		Padding(0, 14).Radius(11).Gap(30).Background(tk.surf).Border(1, tk.bd2).
		AlignItems(ui.Center).Justify(ui.Start)
	trigger.Children(func() {
		ui.Text(c, text(value)).FontSize(13).FontWeight(500).SingleLine().TextColor(tk.text)
		ui.Icon(c, designicons.ChevronDown()).Size(14, 14).TextColor(tk.text)
	})
	open := ui.Local(trigger, "open", func() bool { return false })
	if open == nil {
		return
	}
	if trigger.Clicked() {
		*open = true
	}
	overlayPopover(c, trigger, open, func() {
		ui.Column(c).Gap(2).Padding(4).MinWidth(190).Children(func() {
			for _, v := range choices {
				item := ui.ButtonBase(c).Label(text(v)).FillWidth().Height(32).Padding(0, 12).
					Radius(6).Justify(ui.Start).AlignItems(ui.Center)
				if v == value {
					item.Background(tk.sel)
				} else if item.Hovered() {
					item.Background(tk.text.Alpha(.05))
				}
				item.Children(func() {
					ui.Text(c, text(v)).FontSize(13).SingleLine().TextColor(tk.text)
				})
				if item.Clicked() {
					set(v)
					*open = false
				}
			}
		})
	})
}

// segmentText names a segmented option. System reads as the short "System"
// in the segmented control, not the select's longer "Follow system".
func (u *UI) segmentText(value string) string {
	if value == prefs.System {
		return u.t("optionSystemShort")
	}
	return u.optionText(value)
}

// segment is the Appearance control from the design: a track of equal
// buttons, the selected one a raised card. Values stay the preference's own.
func (u *UI) segment(c *ui.Context, label, value string, choices []string, set func(string)) {
	tk := kitDesignTokens(c.Theme().Dark)
	ui.Row(c).Key("segment:" + label).Shrink(0).Gap(2).Padding(2).Radius(9).Background(tk.fill).Children(func() {
		for _, v := range choices {
			on := v == value
			b := ui.ButtonBase(c).Key("segment:"+label+":"+v).Height(26).Padding(0, 14).Radius(7).
				Center().Label(label + ": " + u.segmentText(v))
			if on {
				b.Background(tk.surf).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
			} else if b.Hovered() {
				b.Background(tk.text.Alpha(.05))
			}
			b.Children(func() {
				txt := ui.Text(c, u.segmentText(v)).FontSize(12).SingleLine()
				if on {
					txt.FontWeight(600).TextColor(tk.text)
				} else {
					txt.TextColor(tk.muted)
				}
			})
			if b.Clicked() && !on {
				set(v)
			}
		}
	})
}

// settingsCategories lists the Settings pages; AI settings add one.
func (u *UI) settingsCategories() []settingsCategory {
	general := settingsCategory{
		ID: "general", Title: u.t("settingsGeneral"),
		Groups: []settingsGroup{
			{ID: "interface", Title: u.t("settingsInterface"), Items: []settingsItem{
				{ID: "language", Title: u.t("settingsLanguage"), Description: u.t("settingsLanguageHint"),
					Keywords: []string{"language", "语言", "English", "中文"}, Build: func(c *ui.Context) {
						u.choice(c, u.t("settingsLanguage"), u.prefs.Language, append([]string{prefs.System}, prefs.Languages...), func(v string) {
							p := u.prefs
							p.Language = v
							u.setPreferences(p)
						})
					}},
				{ID: "appearance", Title: u.t("settingsAppearance"),
					Keywords: []string{"theme", "dark", "light", "外观", "深色", "浅色"}, Build: func(c *ui.Context) {
						u.segment(c, u.t("settingsAppearance"), u.prefs.Appearance, []string{"light", "dark", prefs.System}, func(v string) {
							p := u.prefs
							p.Appearance = v
							u.setPreferences(p)
						})
					}},
			}},
			{ID: "files", Title: u.t("settingsFiles"), Items: []settingsItem{u.kitEditorSetting()}},
			{ID: "commits", Title: u.t("settingsCommits"), Items: []settingsItem{u.kitCommitIdentitySetting()}},
		},
	}
	about := settingsCategory{
		ID: "about", Title: u.t("settingsAbout"),
		Groups: []settingsGroup{{ID: "about", Items: []settingsItem{{
			ID: "about", Title: "GitOwl", Description: u.t("aboutBody"),
			Content: func(c *ui.Context) {
				tk := kitDesignTokens(c.Theme().Dark)
				ui.Column(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(8).Padding(16, 0).Children(func() {
					ui.Image(c, OwlImage()).Size(48, 48)
					ui.Text(c, "GitOwl").Bold().FontSize(18).TextColor(tk.text)
					if u.opts.Version != "" {
						ui.Text(c, u.t("aboutVersion", "version", u.opts.Version)).FontSize(13).TextColor(tk.muted)
					}
					ui.Text(c, u.t("aboutBody")).Wrap().TextAlign(ui.Center).TextColor(tk.muted)
				})
			},
		}}}},
	}
	cats := []settingsCategory{general}
	cats = append(cats, u.extraSettings()...)
	return append(cats, about)
}

// kitCommitIdentitySetting is the switch that shows the commit panel's
// Commit identity line. It is off by default, so the panel is unchanged
// until the user turns it on.
func (u *UI) kitCommitIdentitySetting() settingsItem {
	return settingsItem{ID: "commitIdentity", Title: u.t("settingsCommitIdentity"), Description: u.t("settingsCommitIdentityHint"),
		Keywords: []string{"identity", "author", "name", "email", "user", "身份", "作者", "用户名", "邮箱"}, Build: func(c *ui.Context) {
			value := "off"
			if u.prefs.CommitIdentity {
				value = "on"
			}
			u.segment(c, u.t("settingsCommitIdentity"), value, []string{"off", "on"}, func(v string) {
				p := u.prefs
				p.CommitIdentity = v == "on"
				u.setPreferences(p)
				if p.CommitIdentity {
					u.loadIdentity(false)
				}
			})
		}}
}

// kitEditorSetting is the General row that chooses the editor, drawn with
// kit buttons. Choosing and clearing still go through the existing editor
// helpers so the path is saved as a preference, never a credential.
func (u *UI) kitEditorSetting() settingsItem {
	return settingsItem{ID: "editor", Title: u.t("settingsEditor"), Description: u.t("settingsEditorHint"), Stacked: true,
		Keywords: []string{"editor", "编辑器", "VS Code", "Zed", "Xcode"}, Build: func(c *ui.Context) {
			tk := kitDesignTokens(c.Theme().Dark)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				label := u.t("editorNone")
				if e := u.editor(); e.Set() {
					label = e.Name
					if !u.prefs.Editor.Set() {
						label = u.t("editorDetected", "editor", e.Name)
					}
				}
				text := ui.Text(c, label).SingleLine().Ellipsis("…").MaxWidth(320).Shrink(1).MinWidth(36).TextColor(tk.text)
				if e := u.editor(); e.Set() {
					text.Tooltip(e.Path)
				}
				choose := kitSecondaryButton(c, u.t("chooseEditor"), tk, u.opts.ChooseApplication == nil)
				if choose.Clicked() {
					u.chooseEditor()
				}
				if u.prefs.Editor.Set() {
					if kitIconButton(c, designicons.Close(), u.t("clearEditor"), tk, false).Clicked() {
						p := u.prefs
						p.Editor = prefs.Editor{}
						u.setPreferences(p)
					}
				}
			})
		}}
}

// settingsDialog builds Settings above the window from the HTML design: an
// 800×540 dialog, a 220 nav column and a 580 content panel. Narrow windows
// stack a Category select above the page. Surfaces come from the ticket 01
// kit; the panel is a dialog overlay, not another window, and not glass.
func (u *UI) settingsDialog(c *ui.Context) {
	if !u.set.open {
		return
	}
	ui.Box(c).Key("settingsDialog").Children(func() {
		ui.Overlay(c, func() {
			w, h := c.Size()
			width := min(float32(800), w)
			height := min(float32(540), h)
			narrow := width < 620
			tk := kitDesignTokens(c.Theme().Dark)
			back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).
				Background(ui.RGBA(0, 0, 0, .18)).Modal().Transition(dialogFade)
			clicked := back.Clicked()
			escape := back.OverlayShortcut(0, ui.KeyEscape)
			if clicked || escape {
				u.set.open = false
			}
			back.Children(func() {
				panel := ui.Column(c).Role(ui.RoleDialog).Label(u.t("settings")).
					Absolute().Left(max(0, (w-width)/2)).Top(max(0, (h-height)/2)).
					Width(width).Height(height).MaxWidth(w).MaxHeight(h).
					Padding(0).Gap(0).Radius(18).Background(tk.bg).Clip().
					Shadow(0, 20, 60, 0, ui.RGBA(0, 0, 0, 0.2))
				panel.Clicked()
				panel.Children(func() {
					cats := u.settingsCategories()
					if narrow {
						u.settingsNarrow(c, cats, tk)
						return
					}
					ui.Row(c).Fill().MinWidth(0).Gap(0).Children(func() {
						u.settingsNav(c, cats, tk)
						u.settingsContent(c, cats, tk)
					})
				})
			})
		})
	})
}

// settingsNav is the left column: the "Settings" title, the search box and
// a button per category, the selected one on the kit selected fill.
func (u *UI) settingsNav(c *ui.Context, cats []settingsCategory, tk kitTokens) {
	sel := u.settingsSelected(cats)
	ui.Column(c).Key("settingsNav").Width(220).FillHeight().Shrink(0).Padding(22, 14).
		Background(tk.nav).Children(func() {
		ui.Box(c).Padding(0, 6).Children(func() {
			ui.Text(c, u.t("settings")).FontWeight(700).FontSize(17).TextColor(tk.text)
		})
		u.settingsSearchBox(c, tk)
		for _, cat := range cats {
			u.settingsNavItem(c, cat, cat.ID == sel, tk)
		}
	})
	ui.Box(c).Width(1).FillHeight().Shrink(0).Background(tk.border)
}

// settingsNavItem is one category button: 192×36, radius 9, left-aligned,
// the selected one on the kit selected fill with a 600 label.
func (u *UI) settingsNavItem(c *ui.Context, cat settingsCategory, on bool, tk kitTokens) {
	b := ui.ButtonBase(c).Key("settingsNav:"+cat.ID).FillWidth().Height(36).Padding(0, 10).Radius(9).
		AlignItems(ui.Center).Justify(ui.Start).Label(cat.Title)
	if on {
		b.Background(tk.sel)
	} else if b.Hovered() {
		b.Background(tk.text.Alpha(.05))
	}
	b.Children(func() {
		txt := ui.Text(c, cat.Title).FontSize(13).SingleLine().TextColor(tk.text)
		if on {
			txt.FontWeight(600)
		}
	})
	if b.Clicked() {
		u.set.state.Category = cat.ID
		u.set.state.Query = ""
	}
}

// settingsSearchBox is the nav's search field: 192 wide (the 220 column
// less 14 padding either side), 34 tall, bound to the state's query so a
// search spans every category.
func (u *UI) settingsSearchBox(c *ui.Context, tk kitTokens) {
	ui.Box(c).FillWidth().MinWidth(0).Padding(16, 0, 12, 0).Key("settingsSearch").Children(func() {
		ui.Box(c).FillWidth().Height(34).Children(func() {
			ui.Icon(c, designicons.Search()).Size(14, 14).TextColor(tk.muted).Absolute().Left(10).Top(10)
			ui.TextInputBase(c, &u.set.state.Query).FillWidth().Height(34).Padding(0, 10, 0, 30).
				Radius(10).Border(1, tk.bd2).Background(tk.surf).
				Label(u.t("settingsSearch")).Placeholder(u.t("settingsSearch") + "…")
		})
	})
}

// settingsSelected resolves the selected category ID, defaulting to the
// first when the state holds none or an unknown one.
func (u *UI) settingsSelected(cats []settingsCategory) string {
	for _, cat := range cats {
		if cat.ID == u.set.state.Category {
			return cat.ID
		}
	}
	if len(cats) > 0 {
		return cats[0].ID
	}
	return ""
}

// secTitle is a group / section heading: 11/600, letter-spaced, uppercase
// and muted, matching the design's .sec. MyGo has no text-transform, so
// the text is uppercased here (a no-op for non-Latin scripts).
func (u *UI) secTitle(c *ui.Context, title string, tk kitTokens) {
	ui.Text(c, strings.ToUpper(title)).FontWeight(600).FontSize(11).
		LetterSpacing(.55).TextColor(tk.muted)
}

// settingsContent is the right panel: a header row with the page title and
// the close button, then either the selected category's page or, while a
// query is set, the cross-category results. The save failure, when any,
// sits under it.
func (u *UI) settingsContent(c *ui.Context, cats []settingsCategory, tk kitTokens) {
	searching := strings.TrimSpace(u.set.state.Query) != ""
	ui.Column(c).Key("settingsContent").Grow(1).MinWidth(0).FillHeight().Background(tk.bg).Children(func() {
		ui.Column(c).FillWidth().Grow(1).MinWidth(0).Padding(22, 28).Children(func() {
			title := u.t("settings")
			if searching {
				title = u.t("settingsResults")
			} else if sel := u.settingsSelected(cats); sel != "" {
				for _, cat := range cats {
					if cat.ID == sel {
						title = cat.Title
					}
				}
			}
			u.settingsHeader(c, title, tk)
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).Key("settingsPage:" + u.set.state.Query + ":" + u.set.state.Category).FillWidth().MinWidth(0).Children(func() {
					if searching {
						u.settingsResultsPage(c, cats, false, tk)
					} else {
						u.settingsBody(c, cats, false, tk)
					}
				})
			})
		})
		u.settingsSaveError(c, tk)
	})
}

// settingsHeader is the title row: a 22/700 title on the left and the
// 34×34 close button on the right, holding the kit ✕ glyph.
func (u *UI) settingsHeader(c *ui.Context, title string, tk kitTokens) {
	ui.Row(c).FillWidth().Shrink(0).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontWeight(700).FontSize(22).TextColor(tk.text).Grow(1).MinWidth(0)
		b := ui.ButtonBase(c).Key("settingsClose").Width(34).Height(34).Radius(12).Center().Label(u.t("settingsClose"))
		if b.Hovered() {
			b.Background(tk.text.Alpha(.05))
		}
		b.Children(func() {
			ui.Icon(c, designicons.Close()).Size(16, 16).TextColor(tk.text)
		})
		if b.Clicked() {
			u.set.open = false
		}
	})
}

// settingsBody renders the selected category's groups and items: each
// non-empty group title as an uppercase muted .sec heading, each item as a
// row with a soft divider below (the last row has none), then the footer
// hint on the General page.
func (u *UI) settingsBody(c *ui.Context, cats []settingsCategory, narrow bool, tk kitTokens) {
	sel := u.settingsSelected(cats)
	var cat settingsCategory
	for _, cc := range cats {
		if cc.ID == sel {
			cat = cc
		}
	}
	type row struct {
		group settingsGroup
		first bool
		item  settingsItem
	}
	var rows []row
	for _, g := range cat.Groups {
		for i, it := range g.Items {
			rows = append(rows, row{group: g, first: i == 0, item: it})
		}
	}
	for i, r := range rows {
		if r.first && r.group.Title != "" {
			// The first section label sits 14 below the top; later ones sit
			// 18 below the rows above, as the design's .lb paddings do.
			if i == 0 {
				ui.Box(c).Padding(14, 0, 0, 0).Children(func() { u.secTitle(c, r.group.Title, tk) })
			} else {
				ui.Box(c).Padding(18, 0, 0, 0).Children(func() { u.secTitle(c, r.group.Title, tk) })
			}
		}
		// The AI page's wide controls (text boxes, option lists) stack
		// full width below their text, the design's wide-control rows.
		u.settingsRow(c, r.item, i < len(rows)-1, narrow || cat.ID == "ai", tk)
	}
	if cat.ID == "general" {
		ui.Box(c).Padding(12, 0, 0, 0).Children(func() {
			ui.Text(c, u.t("settingsApplyImmediately")).FontSize(12).TextColor(tk.muted)
		})
	}
}

// settingsRow is one setting: its title (and description) on the left and
// its control on the right, with a 14/0 vertical padding and, unless it is
// the last row, a soft bottom divider. A stacked item (a wide control such
// as a text box or option list, or any item in a narrow window) puts its
// control full width below the text. An item's Content replaces the row.
func (u *UI) settingsRow(c *ui.Context, it settingsItem, divider, stack bool, tk kitTokens) {
	stack = stack || it.Stacked
	ui.Column(c).FillWidth().MinWidth(0).Children(func() {
		switch {
		case it.Content != nil:
			ui.Box(c).FillWidth().MinWidth(0).Padding(18, 0).Children(func() { it.Content(c) })
		case stack:
			ui.Column(c).FillWidth().MinWidth(0).Padding(18, 0).Gap(8).Children(func() {
				u.settingsRowText(c, it, tk)
				if it.Build != nil {
					ui.Box(c).FillWidth().MinWidth(0).Children(func() { it.Build(c) })
				}
			})
		default:
			ui.Row(c).FillWidth().MinWidth(0).Padding(18, 0).Gap(24).AlignItems(ui.Start).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() { u.settingsRowText(c, it, tk) })
				if it.Build != nil {
					ui.Row(c).Shrink(0).AlignItems(ui.Center).Children(func() { it.Build(c) })
				}
			})
		}
		if divider {
			ui.Box(c).FillWidth().Height(1).Shrink(0).Background(tk.border)
		}
	})
}

// settingsRowText is a row's label (14/600) and its description (12 muted,
// at most 330 wide), matching the design's label and p.
func (u *UI) settingsRowText(c *ui.Context, it settingsItem, tk kitTokens) {
	if it.Title != "" {
		ui.Text(c, it.Title).FontWeight(600).FontSize(14).TextColor(tk.text).Wrap()
	}
	if it.Description != "" {
		ui.Text(c, it.Description).FontSize(13).
			TextColor(tk.muted).MaxWidth(330).Wrap()
	}
}

// settingsResultsPage lists every item matching the query across all
// categories, grouped under each category's title, with the no-results
// text when none match.
func (u *UI) settingsResultsPage(c *ui.Context, cats []settingsCategory, narrow bool, tk kitTokens) {
	results := searchSettings(cats, u.set.state.Query)
	if len(results) == 0 {
		ui.Box(c).Padding(14, 0).Children(func() {
			ui.Text(c, u.t("settingsNoResults")).FontSize(14).TextColor(tk.muted).Wrap()
		})
		return
	}
	lastCat := -1
	for n, r := range results {
		cat := cats[r.Category]
		item := cat.Groups[r.Group].Items[r.Item]
		if r.Category != lastCat {
			ui.Box(c).Padding(20, 0, 4, 0).Children(func() { u.secTitle(c, cat.Title, tk) })
			lastCat = r.Category
		}
		u.settingsRow(c, item, n < len(results)-1, narrow, tk)
	}
}

// settingsNarrow stacks the search, category selector and the page in one
// column, for a window too small for the 232 nav beside the content.
func (u *UI) settingsNarrow(c *ui.Context, cats []settingsCategory, tk kitTokens) {
	ui.Column(c).Key("settingsNarrow").Fill().MinWidth(0).Background(tk.bg).Children(func() {
		ui.Column(c).FillWidth().Grow(1).MinWidth(0).Padding(20, 20).Gap(12).Children(func() {
			u.settingsHeader(c, u.t("settings"), tk)
			u.settingsSearchBox(c, tk)
			if strings.TrimSpace(u.set.state.Query) == "" {
				sel := u.settingsSelected(cats)
				titles := make([]string, 0, len(cats))
				ids := make([]string, 0, len(cats))
				for _, cat := range cats {
					ids = append(ids, cat.ID)
					titles = append(titles, cat.Title)
				}
				u.kitChoice(c, u.t("settingsCategory"), sel, ids, func(id string) string {
					for i, v := range ids {
						if v == id {
							return titles[i]
						}
					}
					return id
				}, func(v string) {
					u.set.state.Category = v
				})
			}
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).Key("settingsNarrowPage:" + u.set.state.Query + ":" + u.set.state.Category).FillWidth().MinWidth(0).Children(func() {
					if strings.TrimSpace(u.set.state.Query) != "" {
						u.settingsResultsPage(c, cats, true, tk)
					} else {
						u.settingsBody(c, cats, true, tk)
					}
				})
			})
		})
		u.settingsSaveError(c, tk)
	})
}

// searchSettings matches every item whose title, description, keywords or
// category/group names contain every whitespace-separated query term.
func searchSettings(categories []settingsCategory, query string) []settingsResult {
	terms := strings.Fields(strings.ToLower(query))
	var results []settingsResult
	for ci, category := range categories {
		for gi, group := range category.Groups {
			for ii, item := range group.Items {
				haystack := strings.ToLower(strings.Join([]string{
					category.Title, category.Description, group.Title,
					item.Title, item.Description, strings.Join(item.Keywords, " "),
				}, " "))
				match := true
				for _, term := range terms {
					if !strings.Contains(haystack, term) {
						match = false
						break
					}
				}
				if match {
					results = append(results, settingsResult{ci, gi, ii})
				}
			}
		}
	}
	return results
}

// settingsSaveError shows the preference save failure, when any, under the
// page: the recoverable reason and the underlying error, never a credential.
func (u *UI) settingsSaveError(c *ui.Context, tk kitTokens) {
	if u.set.saveErr == "" {
		return
	}
	ui.Column(c).FillWidth().Shrink(0).Padding(8, 16).Children(func() {
		ui.Text(c, u.t("prefsSaveFailed")).Wrap().TextColor(tk.del)
		ui.Text(c, u.set.saveErr).FontSize(12).Wrap().TextColor(tk.muted)
	})
}
