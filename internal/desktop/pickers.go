package desktop

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Pickers: keyboard lists of where to go, drawn with kit so the File
// menu's Recent and Worktree items still work after Onevra is gone.
const (
	pickRecent   = "recent"
	pickWorktree = "worktree"
)

type picker struct {
	open  bool
	kind  string
	query string
	index int
}

func (u *UI) openPicker(kind string) {
	if kind == pickWorktree {
		u.loadWorktrees()
	}
	u.pick = picker{open: true, kind: kind}
}

const (
	pickOther = "action:open"
	pickClose = "action:close"
)

type pickerRow struct {
	id, title, hint string
	disabled        bool
}

func (u *UI) recentPickerRows() []pickerRow {
	var rows []pickerRow
	current := ""
	if u.repo != nil {
		current = u.repo.Root()
	}
	for _, root := range u.prefs.Recent {
		it := pickerRow{id: root, title: filepath.Base(root), hint: homeShort(root)}
		switch {
		case root == current:
			it.disabled, it.hint = true, u.t("pickCurrent")+" · "+homeShort(root)
		case u.recentMissing(root):
			it.disabled, it.hint = true, u.t("recentMissing")+" · "+homeShort(root)
		}
		rows = append(rows, it)
	}
	rows = append(rows, pickerRow{id: pickOther, title: u.t("openOtherRepository"), disabled: !u.Can(ActionOpen)})
	if u.repo != nil {
		rows = append(rows, pickerRow{id: pickClose, title: u.t("closeRepository"), disabled: !u.Can(ActionClose)})
	}
	return rows
}

func (u *UI) worktreePickerRows() []pickerRow {
	var rows []pickerRow
	for i, w := range u.trees.list {
		it := pickerRow{id: strconv.Itoa(i), title: w.Name(), hint: u.worktreeState(w) + " · " + homeShort(w.Path)}
		switch {
		case w.Current:
			it.disabled, it.hint = true, u.t("pickCurrent")+" · "+it.hint
		case w.Prunable:
			it.disabled, it.hint = true, u.t("worktreeMissing")+" · "+it.hint
		}
		rows = append(rows, it)
	}
	return rows
}

func (u *UI) pickerVisible(rows []pickerRow) []pickerRow {
	q := strings.ToLower(strings.TrimSpace(u.pick.query))
	if q == "" {
		return rows
	}
	var out []pickerRow
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r.title), q) || strings.Contains(strings.ToLower(r.hint), q) {
			out = append(out, r)
		}
	}
	return out
}

func (u *UI) pickerDialog(c *ui.Context) {
	p := &u.pick
	if !p.open {
		return
	}
	var rows []pickerRow
	label, placeholder := u.t("openRecentMenu"), u.t("pickRecentPlaceholder")
	if p.kind == pickWorktree {
		rows, label, placeholder = u.worktreePickerRows(), u.t("switchWorktree"), u.t("pickWorktreePlaceholder")
	} else {
		rows = u.recentPickerRows()
	}
	shown := u.pickerVisible(rows)
	if p.index >= len(shown) {
		p.index = max(0, len(shown)-1)
	}
	tk := kitDesignTokens(c.Theme().Dark)
	ui.Box(c).Key("picker").Children(func() {
		ui.Overlay(c, func() {
			w, h := c.Size()
			width := min(float32(480), w-40)
			back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).
				Background(ui.RGBA(0, 0, 0, .18)).Modal().Transition(dialogFade)
			if back.Clicked() || back.OverlayShortcut(0, ui.KeyEscape) {
				p.open = false
			}
			if back.OverlayShortcut(0, ui.KeyEnter) && p.index >= 0 && p.index < len(shown) {
				u.runPicker(shown[p.index])
			}
			if back.OverlayShortcut(0, ui.KeyDown) && len(shown) > 0 {
				p.index = (p.index + 1) % len(shown)
			}
			if back.OverlayShortcut(0, ui.KeyUp) && len(shown) > 0 {
				p.index = (p.index - 1 + len(shown)) % len(shown)
			}
			back.Children(func() {
				panel := ui.Column(c).Role(ui.RoleDialog).Label(label).
					Absolute().Left(max(0, (w-width)/2)).Top(max(float32(80), (h-360)/2)).
					Width(width).MaxHeight(min(float32(420), h-80)).Padding(16).Gap(10).
					Radius(14).Background(tk.bg).Clip().
					Shadow(0, 20, 60, 0, ui.RGBA(0, 0, 0, 0.25))
				panel.Clicked()
				panel.Children(func() {
					ui.Text(c, label).FontWeight(600).FontSize(15).TextColor(tk.text)
					field := ui.TextInput(c, &p.query).FillWidth().Height(kitFieldH).
						Label(label).Placeholder(placeholder).AutoFocus()
					if field.Changed() {
						p.index = 0
					}
					ui.Scroll(c).Fill().Children(func() {
						if len(shown) == 0 {
							ui.Text(c, u.t("pickNoMatch")).FontSize(13).TextColor(tk.muted)
							return
						}
						for i, row := range shown {
							on := i == p.index
							bg := ui.Color{}
							if on {
								bg = tk.sel
							}
							b := ui.ButtonBase(c).FillWidth().Height(36).Padding(0, 10).Radius(8).
								Background(bg).Label(row.title).Disabled(row.disabled).Justify(ui.Start).AlignItems(ui.Center)
							b.Children(func() {
								ui.Column(c).Gap(2).Children(func() {
									ui.Text(c, row.title).FontSize(13).TextColor(tk.text).SingleLine().Ellipsis("…")
									if row.hint != "" {
										ui.Text(c, row.hint).FontSize(12).TextColor(tk.muted).SingleLine().Ellipsis("…")
									}
								})
							})
							if b.Clicked() && !row.disabled {
								u.runPicker(row)
							}
						}
					})
				})
			})
		})
	})
}

func (u *UI) runPicker(row pickerRow) {
	u.pick.open = false
	if u.pick.kind == pickWorktree {
		i, err := strconv.Atoi(row.id)
		if err != nil || i >= len(u.trees.list) {
			return
		}
		u.openWorktree(u.trees.list[i])
		return
	}
	switch row.id {
	case pickOther:
		u.Do(ActionOpen)
	case pickClose:
		u.Do(ActionClose)
	default:
		u.openRecent(row.id)
	}
}
