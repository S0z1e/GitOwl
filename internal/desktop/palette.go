package desktop

import (
	"context"
	"gitowl/internal/desktop/designicons"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// paletteState is the command palette opened with ⌘K: an empty field and
// the full action list, filtered by a case-insensitive title substring.
type paletteState struct {
	open   bool
	query  string
	cursor int
}

type paletteRow struct {
	action      Action
	title, hint string
	keys        string
	color       ui.Color
	icon        *ui.SVG
}

type paletteGroup struct {
	title string
	rows  []paletteRow
}

const (
	palettePanelW  = 760
	palettePanelH  = 500
	paletteHeaderH = 56
	paletteFooterH = 40
	paletteRowH    = 34
)

// paletteActions is the command palette's rows, in artboard order.
var paletteActions = []Action{
	ActionCommit, ActionPush, ActionPull,
	ActionEditor, ActionTerminal, ActionFinder, ActionSettings,
	ActionStash, ActionDiscardUnstaged,
}

// paletteKeys are the seven shortcuts the custom shell runs, besides the
// palette's own ⌘K, Enter and Esc.
var paletteKeys = map[Action]struct {
	mods ui.Modifiers
	key  ui.Key
}{
	ActionCommit:   {ui.Cmd, ui.KeyEnter},
	ActionPush:     {ui.Cmd | ui.Shift, ui.KeyP},
	ActionPull:     {ui.Cmd | ui.Shift, ui.KeyL},
	ActionEditor:   {ui.Cmd, ui.KeyE},
	ActionTerminal: {ui.Cmd | ui.Alt, ui.KeyT},
	ActionFinder:   {ui.Cmd | ui.Shift, ui.KeyR},
	ActionSettings: {ui.Cmd, ui.KeyComma},
}

func (u *UI) togglePalette() {
	if u.chrome.pal.open {
		u.closePalette()
		return
	}
	u.chrome.pal = paletteState{open: true}
}

func (u *UI) closePalette() { u.chrome.pal = paletteState{} }

func (u *UI) paletteTitle(a Action) string {
	switch a {
	case ActionCommit:
		return u.t("paletteCommit", "count", len(u.snap.Staged))
	case ActionPush:
		return u.t("palettePush")
	case ActionPull:
		return u.t("palettePull")
	case ActionEditor:
		return u.t("paletteEditor")
	case ActionTerminal:
		return u.t("paletteTerminal")
	case ActionFinder:
		return u.t("paletteFinder")
	case ActionSettings:
		return u.t("paletteSettings")
	case ActionStash:
		return u.t("paletteStash")
	case ActionDiscardUnstaged:
		return u.t("paletteDiscard")
	}
	return ""
}

func (u *UI) paletteHint(a Action) string {
	switch a {
	case ActionCommit:
		if u.snap.Detached {
			return u.t("detachedHead")
		}
		return u.snap.Branch
	case ActionPush:
		if r := u.sync.info.UpstreamRemote; r != "" {
			return r
		}
		return u.sync.info.DefaultFetch()
	case ActionPull:
		return u.chromeSyncTarget()
	case ActionDiscardUnstaged:
		return u.t("changedFiles", "count", len(u.snap.Unstaged))
	}
	return ""
}

func paletteKeyGlyph(a Action) string {
	switch a {
	case ActionCommit:
		return "⌘↵"
	case ActionPush:
		return "⇧⌘P"
	case ActionPull:
		return "⇧⌘L"
	case ActionEditor:
		return "⌘E"
	case ActionTerminal:
		return "⌥⌘T"
	case ActionFinder:
		return "⇧⌘R"
	case ActionSettings:
		return "⌘,"
	}
	return ""
}

func paletteSwatch(a Action) (ui.Color, *ui.SVG) {
	switch a {
	case ActionCommit:
		return ui.Hex("#2563EB"), designicons.Commits()
	case ActionPush:
		return ui.Hex("#1F9D62"), designicons.Push()
	case ActionPull:
		return ui.Hex("#2F6FDD"), designicons.Pull()
	case ActionEditor:
		return ui.Hex("#6B5BD6"), designicons.Editor()
	case ActionTerminal:
		return ui.Hex("#1F9D62"), designicons.Terminal()
	case ActionFinder:
		return ui.Hex("#7A63E6"), designicons.Finder()
	case ActionSettings:
		return ui.Hex("#6B7280"), designicons.Settings()
	case ActionStash:
		return ui.Hex("#D89614"), designicons.Stash()
	case ActionDiscardUnstaged:
		return ui.Hex("#C93C32"), designicons.Close()
	}
	return ui.Hex("#6B7280"), designicons.Settings()
}

func (u *UI) paletteRow(a Action) paletteRow {
	c, icon := paletteSwatch(a)
	return paletteRow{
		action: a,
		title:  u.paletteTitle(a),
		hint:   u.paletteHint(a),
		keys:   paletteKeyGlyph(a),
		color:  c,
		icon:   icon,
	}
}

func paletteMatch(title, q string) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(title), strings.ToLower(q))
}

func (u *UI) paletteGroups() []paletteGroup {
	q := u.chrome.pal.query
	filter := func(actions ...Action) []paletteRow {
		var rows []paletteRow
		for _, a := range actions {
			row := u.paletteRow(a)
			if paletteMatch(row.title, q) {
				rows = append(rows, row)
			}
		}
		return rows
	}
	var groups []paletteGroup
	for _, g := range []paletteGroup{
		{u.t("paletteSuggestions"), filter(ActionCommit, ActionPush, ActionPull)},
		{u.t("paletteRepository"), filter(ActionEditor, ActionTerminal, ActionFinder, ActionSettings)},
		{u.t("paletteGit"), filter(ActionStash, ActionDiscardUnstaged)},
	} {
		if len(g.rows) > 0 {
			groups = append(groups, g)
		}
	}
	return groups
}

func (u *UI) paletteVisible() []paletteRow {
	var rows []paletteRow
	for _, g := range u.paletteGroups() {
		rows = append(rows, g.rows...)
	}
	return rows
}

func (u *UI) runPaletteRow() {
	rows := u.paletteVisible()
	i := u.chrome.pal.cursor
	if i < 0 || i >= len(rows) {
		return
	}
	a := rows[i].action
	if !u.Can(a) {
		return
	}
	u.closePalette()
	u.Do(a)
}

func (u *UI) movePalette(delta int) {
	n := len(u.paletteVisible())
	if n == 0 {
		u.chrome.pal.cursor = 0
		return
	}
	u.chrome.pal.cursor += delta
	if u.chrome.pal.cursor < 0 {
		u.chrome.pal.cursor = 0
	}
	if u.chrome.pal.cursor >= n {
		u.chrome.pal.cursor = n - 1
	}
}

func (u *UI) paletteDialog(c *ui.Context) {
	if !u.chrome.pal.open {
		return
	}
	visible := u.paletteVisible()
	if n := len(visible); u.chrome.pal.cursor >= n {
		u.chrome.pal.cursor = max(0, n-1)
	}
	ui.Box(c).Key("commandPalette").Children(func() {
		ui.Overlay(c, func() {
			w, h := c.Size()
			width := min(float32(palettePanelW), w-32)
			height := min(float32(palettePanelH), h-32)
			tk := kitDesignTokens(c.Theme().Dark)
			back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).
				Background(ui.RGBA(0, 0, 0, .18)).Modal().Transition(dialogFade)
			if back.Clicked() || back.OverlayShortcut(0, ui.KeyEscape) {
				u.closePalette()
			}
			if back.OverlayShortcut(0, ui.KeyEnter) {
				u.runPaletteRow()
			}
			if back.OverlayShortcut(0, ui.KeyUp) {
				u.movePalette(-1)
			}
			if back.OverlayShortcut(0, ui.KeyDown) {
				u.movePalette(1)
			}
			back.Children(func() {
				panel := ui.Column(c).Role(ui.RoleDialog).Label(u.t("commandPalette")).
					Absolute().Left(max(0, (w-width)/2)).Top(max(0, (h-height)/2)).
					Width(width).Height(height).MaxWidth(w).MaxHeight(h).
					Radius(16).Background(tk.surf).Clip().Border(1, tk.border).
					Shadow(0, 20, 60, 0, ui.RGBA(0, 0, 0, 0.2))
				panel.Clicked()
				panel.Children(func() {
					u.paletteHeader(c, tk)
					ui.Column(c).FillWidth().Grow(1).MinHeight(0).Padding(0, 8).Clip().Children(func() {
						index := 0
						for _, g := range u.paletteGroups() {
							u.paletteGroupHead(c, tk, g.title)
							for _, row := range g.rows {
								u.paletteItem(c, tk, row, index == u.chrome.pal.cursor, index)
								index++
							}
						}
					})
					u.paletteFooter(c, tk)
				})
			})
		})
	})
}

func (u *UI) paletteHeader(c *ui.Context, tk kitTokens) {
	ui.Row(c).FillWidth().Height(paletteHeaderH).Padding(0, 18).Gap(12).
		AlignItems(ui.Center).Border(1, tk.border).Children(func() {
		ui.Text(c, "›").FontSize(16).TextColor(tk.acx)
		field := ui.TextInputBase(c, &u.chrome.pal.query).Grow(1).MinWidth(0).FillWidth().
			Height(paletteHeaderH).FontSize(16).Label(u.t("commandPalette")).AutoFocus()
		field.OnTextKey(func(k ui.TextKey) bool {
			switch {
			case k.Key == ui.KeyEnter && k.Mods == 0:
				u.runPaletteRow()
				return true
			case k.Key == ui.KeyEscape:
				u.closePalette()
				return true
			case k.Key == ui.KeyUp:
				u.movePalette(-1)
				return true
			case k.Key == ui.KeyDown:
				u.movePalette(1)
				return true
			}
			return false
		})
		paletteKbd(c, u.t("paletteEsc"), tk)
	})
}

func (u *UI) paletteGroupHead(c *ui.Context, tk kitTokens, title string) {
	ui.Box(c).FillWidth().Padding(12, 10, 6).Children(func() {
		ui.Text(c, title).FontSize(11).FontWeight(600).LetterSpacing(.05).TextColor(tk.muted)
	})
}

func (u *UI) paletteItem(c *ui.Context, tk kitTokens, row paletteRow, on bool, index int) {
	off := !u.Can(row.action)
	weight := 500
	bg := ui.Color{}
	if on {
		bg = tk.sel
		weight = 600
	}
	b := ui.ButtonBase(c).FillWidth().Height(paletteRowH).Padding(0, 10).Gap(12).
		Radius(10).AlignItems(ui.Center).Background(bg).Label(row.title)
	if off {
		b.Opacity(.45)
	}
	b.Children(func() {
		ui.Box(c).Size(20, 20).Radius(6).Background(row.color).Center().Shrink(0).Children(func() {
			ui.Icon(c, row.icon).Size(12, 12).TextColor(ui.Hex("#FFFFFF"))
		})
		ui.Text(c, row.title).FontWeight(weight).FontSize(13).SingleLine().Ellipsis("…")
		if row.hint != "" {
			ui.Text(c, row.hint).FontSize(13).TextColor(tk.muted).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		}
		ui.Spacer(c)
		if row.keys != "" {
			paletteKbd(c, row.keys, tk)
		}
	})
	if b.Clicked() {
		u.chrome.pal.cursor = index
		u.runPaletteRow()
	}
}

func (u *UI) paletteFooter(c *ui.Context, tk kitTokens) {
	ui.Row(c).FillWidth().Height(paletteFooterH).Padding(0, 16).Gap(14).
		AlignItems(ui.Center).Border(1, tk.border).Children(func() {
		ui.Text(c, "GitOwl").FontSize(12).TextColor(tk.muted)
		ui.Spacer(c)
		ui.Text(c, u.t("paletteRun")).FontSize(12)
		paletteKbd(c, "↵", tk)
		ui.Text(c, u.t("paletteActions")).FontSize(12)
		paletteKbd(c, "⌘K", tk)
	})
}

func paletteKbd(c *ui.Context, s string, tk kitTokens) {
	ui.Box(c).Padding(1, 6).Radius(5).Background(tk.fill).Border(1, tk.bd2).Shrink(0).Children(func() {
		ui.Text(c, s).Font("monospace").FontSize(11).TextColor(tk.muted).SingleLine()
	})
}

// discardUnstaged restores every tracked unstaged file to its staged
// version, keeping the index, without a confirmation.
func (u *UI) discardUnstaged() {
	repo := u.repo
	if repo == nil || u.writing() || len(u.snap.Unstaged) == 0 {
		return
	}
	d := &u.discard
	d.seq++
	seq := d.seq
	d.busy, d.err = true, nil
	u.background(func() func() {
		snap, err := repo.DiscardUnstaged(context.Background())
		return func() {
			if repo != u.repo || seq != u.discard.seq {
				return
			}
			u.discard.busy = false
			if snap.Root != "" {
				if sel := u.wt.selected; sel != nil && sel.area == workflow.Unstaged && !inArea(snap, sel.area, sel.path) {
					u.wt.selected = u.followFile(snap, workflow.Change{Path: sel.path, Area: workflow.Unstaged})
				}
				u.setSnapshot(snap)
			}
			if err != nil {
				u.report(err)
				return
			}
			u.notice = nil
			u.toast("success", u.t("discardedUnstagedToast"))
		}
	})
}
