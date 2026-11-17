package desktop

import (
	"gitowl/internal/desktop/designicons"
	"path"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

func (u *UI) chromeAreaTitle(a workflow.Area) string {
	switch a {
	case workflow.Staged:
		return u.t("chromeStagedChanges")
	case workflow.Unstaged:
		return u.t("chromeUnstagedChanges")
	default:
		return u.t("chromeUntrackedFiles")
	}
}

// chromeEnsureFileSelection keeps a file selected while the Diff column is
// on screen. A selection that left the list is replaced by the next file,
// or cleared when Filter files hides every row.
func (u *UI) chromeEnsureFileSelection() {
	if u.repo == nil || u.middleTab != 0 || u.lay.narrow {
		return
	}
	if u.wt.selected != nil {
		if _, ok := u.wt.row(*u.wt.selected); ok {
			return
		}
	}
	if i := u.wt.fileNear(0, true); i >= 0 {
		u.selectRow(i)
		return
	}
	u.wt.diffSeq++
	u.wt.selected, u.wt.diff, u.wt.cursor = nil, nil, -1
}

// chromeChanges draws Current changes: Filter files above the list, then the
// three areas that currently have files. Stage All and Unstage All sit in the
// area headers, as the design draws them.
func (u *UI) chromeChanges(c *ui.Context) {
	tk := kitDesignTokens(c.Theme().Dark)
	ui.Column(c).Fill().MinWidth(0).Background(tk.bg).Children(func() {
		u.chromeFilterBar(c, tk)
		ui.Box(c).Height(10).Shrink(0)
		u.chromeSkipped(c, tk)
		ui.Column(c).Grow(1).FillWidth().MinWidth(0).MinHeight(0).Children(func() {
			if u.showClean() {
				u.chromeCleanState(c, tk)
				return
			}
			u.chromeChangesList(c, tk)
		})
	})
}

// chromeCleanState is what Current changes shows when the working tree has
// no changes: an icon with the clean title and body.
func (u *UI) chromeCleanState(c *ui.Context, tk kitTokens) {
	ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
		ui.Box(c).Size(48, 48).Radius(24).Background(tk.accent.Alpha(.1)).Center().Shrink(0).Children(func() {
			ui.Icon(c, designicons.Check()).Size(22, 22).TextColor(tk.acx)
		})
		ui.Text(c, u.t("cleanTitle")).FontWeight(600).FontSize(15).TextColor(tk.text).SingleLine()
		ui.Text(c, u.t("cleanBody")).FontSize(13).TextColor(tk.muted).Wrap().TextAlign(ui.Center)
	})
}

func (u *UI) chromeFilterBar(c *ui.Context, tk kitTokens) {
	ui.Row(c).FillWidth().Height(kitFieldH).Padding(8, 12, 0).Shrink(0).Children(func() {
		ui.Row(c).FillWidth().Height(kitFieldH).Radius(8).Gap(8).
			Background(tk.surf).Border(1, tk.bd2).Padding(0, 12).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, designicons.Search()).Size(14, 14).TextColor(tk.muted)
			text := u.wt.filter
			field := ui.TextInputBase(c, &text).Grow(1).MinWidth(0).FillWidth().
				Label(u.t("changesFilter")).Placeholder(u.t("changesFilterPlaceholder"))
			u.chromeMarkFocus(field)
			if field.Changed() {
				u.setFilter(text)
			}
			if u.wt.filter != "" {
				clear := ui.ButtonBase(c).Size(22, 22).Radius(6).Label(u.t("changesFilterClear"))
				clear.Children(func() {
					ui.Icon(c, designicons.Close()).Size(12, 12).TextColor(tk.muted)
				})
				if clear.Clicked() {
					u.setFilter("")
				}
			}
		})
	})
}

func (u *UI) chromeBulkButton(c *ui.Context, tk kitTokens, unstage bool) {
	label := u.t("stageAll")
	hint := u.t("stageAllHint")
	block := u.stageAllBlock()
	if unstage {
		label = u.t("unstageAll")
		hint = u.t("unstageAllHint")
		block = u.unstageAllBlock()
	}
	off := block != "" || u.wt.bulk != ""
	tip := hint
	if block != "" && block != "busy" {
		tip = u.t(block)
	}
	// Disabled controls hide tooltips, so the reason stays on a live
	// button whose click is ignored while it cannot run.
	b := kitPillButton(c, label, tk, false).Tooltip(tip).Description(tip)
	// Only a standing reason dims the button. A write in flight does not:
	// dimming on every stage made the header flash, and the click is
	// ignored meanwhile.
	if block != "" && block != "busy" {
		b.Opacity(.45)
	}
	u.chromeMarkFocus(b)
	if b.Clicked() && !off {
		u.bulkStage(unstage)
	}
}

func (u *UI) chromeSkipped(c *ui.Context, tk kitTokens) {
	n := len(u.wt.skipped)
	if n == 0 {
		return
	}
	ui.Row(c).FillWidth().Padding(0, 12, 4).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, u.t("bulkSkipped", "count", n)).FontSize(12).TextColor(tk.muted).Wrap().Grow(1).MinWidth(0)
		d := ui.ButtonBase(c).Label(u.t("dismiss")).Padding(4, 6).Radius(8)
		d.Children(func() {
			ui.Text(c, u.t("dismiss")).FontSize(12).TextColor(tk.muted)
		})
		if d.Clicked() {
			u.wt.skipped = nil
		}
	})
}

func (u *UI) chromeChangesList(c *ui.Context, tk kitTokens) {
	w := &u.wt
	rows := w.chromeRows()
	w.chromeSel = -1
	if w.cursor >= 0 && w.cursor < len(w.rows) {
		key := w.rows[w.cursor].key
		for i, r := range rows {
			if r.key == key {
				w.chromeSel = i
				break
			}
		}
	}
	w.chromeList.Selected = &w.chromeSel
	w.chromeList.Key = func(i int) any { return rows[i].key }
	w.chromeList.Label = func(i int) string {
		if r := rows[i]; r.key.header {
			return u.chromeAreaTitle(r.key.area)
		}
		return rows[i].key.path
	}
	w.chromeList.Header = func(i int) bool { return rows[i].key.header }
	list := ui.List(c, &w.chromeList, len(rows), func(i int) {
		r := rows[i]
		if r.key.header {
			u.chromeAreaHeader(c, tk, r)
			return
		}
		u.chromeChangeRow(c, tk, r.change, w.chromeSel == i)
	}).Fill()
	list.Label(u.t("changesColumn"))
	list.OnShortcut(0, ui.KeySpace, u.toggleSelected)
	u.focusReturn(list, "changesList")
	if list.Changed() && w.chromeSel >= 0 && w.chromeSel < len(rows) {
		r := rows[w.chromeSel]
		if !r.key.header {
			if i, ok := w.row(r.key); ok {
				u.selectRow(i)
			}
		}
	}
	if w.refocus {
		w.refocus = false
		list.Focus()
	}
	w.listFocused = list.FocusVisible()
}

// chromeAreaHeader draws an area's title and count, and its bulk action on the
// right: Unstage All on Staged, Stage All on Unstaged, and Stage All on
// Untracked when no unstaged file gives it a header.
func (u *UI) chromeAreaHeader(c *ui.Context, tk kitTokens, r changeRow) {
	// The header has its own background so rows scrolling under it stay hidden,
	// and it reads "STAGED · 1" as the design's section label does.
	title := strings.ToUpper(u.chromeAreaTitle(r.key.area)) + " · " + strconv.Itoa(r.count)
	ui.Row(c).FillWidth().Padding(14, 12, 6).Gap(8).AlignItems(ui.Center).Background(tk.bg).Children(func() {
		ui.Text(c, title).FontSize(11).FontWeight(600).LetterSpacing(.05).TextColor(tk.muted).SingleLine()
		ui.Box(c).Grow(1)
		switch r.key.area {
		case workflow.Staged:
			u.chromeBulkButton(c, tk, true)
		case workflow.Unstaged:
			u.chromeBulkButton(c, tk, false)
		case workflow.Untracked:
			if len(areaChanges(u.snap, workflow.Unstaged)) == 0 {
				u.chromeBulkButton(c, tk, false)
			}
		}
	})
}

func (u *UI) chromeChangeRow(c *ui.Context, tk kitTokens, ch workflow.Change, selected bool) {
	dir, file := path.Split(ch.Path)
	dir = strings.TrimSuffix(dir, "/")
	twoLine := u.lay.narrow && dir != ""
	height := float32(changeRowHeight)
	if twoLine {
		height = twoLineRowHeight
	}
	bg := ui.Color{}
	if selected {
		bg = tk.sel
	}
	row := ui.Row(c).FillWidth().MinWidth(0).Height(height).Padding(0, 10).Gap(10).
		AlignItems(ui.Center).Background(bg).Radius(10)
	if u.opts.Launch != nil || ch.Area == workflow.Unstaged {
		row.ContextMenu(func(m *ui.Menu) { u.changeItems(m, ch) })
	}
	row.Children(func() {
		if selected && u.wt.listFocused {
			ui.Box(c).Width(2).FillHeight().Background(tk.accent).Shrink(0)
		}
		u.chromeStageBox(c, tk, ch)
		u.chromeStatusLetter(c, tk, ch)
		names := ui.Row(c).Gap(6).AlignItems(ui.Center)
		if twoLine {
			names = ui.Column(c).Gap(1)
		}
		names.Grow(1).MinWidth(0).Children(func() {
			name := ui.Text(c, file).FontSize(13).TextColor(tk.text).SingleLine().Ellipsis("…").Shrink(1).MinWidth(32)
			name.Tooltip(ch.Path)
			if dir != "" {
				ui.Text(c, dir).FontSize(12).TextColor(tk.muted).SingleLine().Ellipsis("…").Shrink(1000).MinWidth(0)
			}
		})
		if partial(u.snap, ch) {
			other := u.t("partialInUnstaged")
			if ch.Area == workflow.Unstaged {
				other = u.t("partialInStaged")
			}
			mark := ui.Text(c, u.t("partialMark")).FontSize(11).TextColor(tk.muted).SingleLine().Shrink(0).
				Padding(0, 6).Radius(4).Border(1, tk.border).Label(u.t("partialMark") + ": " + other)
			mark.Tooltip(other)
		}
	})
}

// chromeStatusLetter draws the design's 20-pixel status badge: M blue, U
// green, A and D tinted with the diff colours, anything else muted.
// chromeBadgeColors returns a status badge's fill and letter colour: M blue,
// U green, A and D tinted with the diff colours, anything else muted.
func chromeBadgeColors(tk kitTokens, letter string) (bg, fg ui.Color) {
	bg, fg = tk.fill, tk.muted
	switch letter {
	case "M":
		bg, fg = tk.mbg, tk.mfg
	case "U":
		bg, fg = tk.ubg, tk.ufg
	case "A":
		bg, fg = tk.addbg, tk.add
	case "D":
		bg, fg = tk.delbg, tk.del
	}
	return bg, fg
}

// chromeStatusBadge draws the design's 20-pixel status badge for a letter.
func chromeStatusBadge(c *ui.Context, tk kitTokens, letter string) {
	bg, fg := chromeBadgeColors(tk, letter)
	ui.Box(c).Size(20, 20).Radius(6).Background(bg).Center().Shrink(0).Children(func() {
		ui.Text(c, letter).FontSize(10).FontWeight(700).TextColor(fg).SingleLine()
	})
}

func (u *UI) chromeStatusLetter(c *ui.Context, tk kitTokens, ch workflow.Change) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Shrink(0).Label(u.t(kindKey(ch))).Tooltip(u.t(kindKey(ch))).Children(func() {
		chromeStatusBadge(c, tk, ch.Letter())
		if ch.Conflicted || ch.Letter() == "!" {
			dot := ui.Box(c).Size(6, 6).Radius(3).Background(tk.warn).Shrink(0)
			dot.Label(u.t("kindX")).Tooltip(u.t("kindX"))
		}
	})
}

// chromeStageBox draws the file's check box. It stays in place while a Git
// change runs: the box is not blanked or dimmed, so the list does not flash
// on every stage. A click during a change is ignored.
func (u *UI) chromeStageBox(c *ui.Context, tk kitTokens, ch workflow.Change) {
	if !canStage(ch) {
		ui.Box(c).Size(16, 16).Shrink(0)
		return
	}
	on := ch.Area == workflow.Staged
	label := u.t("stageFile", "path", ch.Path)
	if on {
		label = u.t("unstageFile", "path", ch.Path)
	}
	box := kitCheckbox(c, &on, label, tk, false)
	if box.Clicked() && !u.indexBusy() {
		u.wt.refocus = true
		u.toggleStaged(ch)
	}
}
