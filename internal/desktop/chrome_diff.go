package desktop

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// chromeDiff draws the Diff column: the file name as the title, Unified /
// Split as one segmented control, Wrap lines, then the existing design
// viewer. There is no column titled Diff.
func (u *UI) chromeDiff(c *ui.Context) {
	tk := kitDesignTokens(c.Theme().Dark)
	ui.Column(c).Fill().MinWidth(0).Background(tk.bg).Children(func() {
		p := u.wt.diff
		if p == nil {
			return
		}
		u.chromeDiffHeader(c, p, tk)
		u.chromeHunkBar(c, p, tk)
		u.diffBodyDesign(c, p)
	})
}

func (u *UI) chromeDiffHeader(c *ui.Context, p *diffPane, tk kitTokens) {
	dir, file := splitPath(p.key.path)
	ui.Column(c).FillWidth().Shrink(0).Padding(10, 14).Gap(8).Children(func() {
		if u.lay.narrow {
			back := kitSecondaryButton(c, u.t("chromeBackToList"), tk, false).Height(24)
			u.chromeMarkFocus(back)
			if back.Clicked() {
				u.lay.detail = false
			}
		}
		if u.lay.narrow {
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				name := ui.Text(c, file).Bold().FontSize(16).TextColor(tk.text).SingleLine().Ellipsis("…")
				name.Tooltip(p.key.path)
				if dir != "" {
					ui.Text(c, strings.TrimSuffix(dir, "/")).FontSize(12).TextColor(tk.muted).SingleLine().Ellipsis("…")
				}
			})
			u.chromeDiffControls(c, p, tk)
			return
		}
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if dir != "" {
				ui.Text(c, dir).FontSize(12).TextColor(tk.muted).SingleLine().Shrink(0)
			}
			name := ui.Text(c, file).Bold().FontSize(16).TextColor(tk.text).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
			name.Tooltip(p.key.path)
			if o := p.info.Change.OrigPath; o != "" {
				ui.Text(c, u.t("renamedFrom", "path", o)).FontSize(12).TextColor(tk.muted).SingleLine().Ellipsis("…").Shrink(2).MinWidth(0)
			}
			if p.state != nil {
				added, deleted := p.state.Stats()
				ui.Text(c, "+"+strconv.Itoa(added)).Font("monospace").FontSize(12).TextColor(tk.add).Shrink(0)
				ui.Text(c, "−"+strconv.Itoa(deleted)).Font("monospace").FontSize(12).TextColor(tk.del).Shrink(0)
			}
			ui.Spacer(c)
			u.chromeDiffControls(c, p, tk)
		})
	})
}

func (u *UI) chromeDiffControls(c *ui.Context, p *diffPane, tk kitTokens) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Wrap().Children(func() {
		mode := 0
		if u.diffSplitActive() {
			mode = 1
		}
		seg := ui.Segmented(c, &mode, u.t("diffUnified"), u.t("diffSplit")).Height(28)
		u.chromeMarkFocus(seg)
		if !u.lay.narrow {
			u.diffSplit = mode == 1
		}
		wrap := kitSecondaryButton(c, u.t("diffWrapLines"), tk, false).Height(28)
		wrap.Tooltip(u.t("diffWrapLinesHint"))
		u.chromeMarkFocus(wrap)
		if wrap.Clicked() {
			u.diffWrap = !u.diffWrap
		}
		if u.opts.Launch != nil || p.key.area == workflow.Unstaged {
			i, ok := u.wt.row(p.key)
			if ok {
				ch := u.wt.rows[i].change
				more := ui.ButtonBase(c).Size(28, 28).Radius(12).
					Background(tk.surf).Border(1, tk.bd2).Label(u.t("fileActions", "path", ch.Path))
				more.Children(func() {
					ui.Text(c, "…").FontSize(16).TextColor(tk.text).Padding(0, 0, 6, 0)
				})
				u.chromeMarkFocus(more)
				u.focusReturn(more, "fileActions")
				more.Menu(func(m *ui.Menu) {
					u.diffHunkItems(m, p)
					u.changeItems(m, ch)
				})
			}
		}
	})
}

func (u *UI) chromeHunkBar(c *ui.Context, p *diffPane, tk kitTokens) {
	if note := u.chromeHunkNote(p); note != "" {
		ui.Text(c, note).FontSize(12).TextColor(tk.muted).Wrap().Padding(0, 14, 8)
		return
	}
	if p.state == nil || !p.state.HunkActions {
		return
	}
	n := len(p.state.Hunks())
	if n == 0 {
		return
	}
	if i, inside := p.state.CurrentHunk(); inside && i != p.caretHunk {
		p.caretHunk, p.hunk = i, i
	} else if !inside {
		p.caretHunk = -1
	}
	p.hunk = max(0, min(p.hunk, n-1))
}

// goToHunk moves the diff to hunk i, wrapping at either end, and brings its
// card into view.
func (u *UI) goToHunk(p *diffPane, i int) {
	n := len(p.state.Hunks())
	if n == 0 {
		return
	}
	p.hunk = (i + n) % n
	p.scrollHunk = p.hunk
	p.caretHunk = p.hunk
	p.state.GoToHunk(p.hunk)
}

// diffHunkItems adds Previous Hunk and Next Hunk to the diff's … menu when
// the diff has more than one hunk. The design draws no step bar, so the menu
// is where a command moves between hunks.
func (u *UI) diffHunkItems(m *ui.Menu, p *diffPane) {
	if p.state == nil || len(p.state.Hunks()) < 2 {
		return
	}
	if m.Item(u.t("previousHunk")).Chosen() {
		u.goToHunk(p, p.hunk-1)
	}
	if m.Item(u.t("nextHunk")).Chosen() {
		u.goToHunk(p, p.hunk+1)
	}
	m.Separator()
}

func (u *UI) chromeHunkNote(p *diffPane) string {
	if p.info.NoHunks != "" {
		return u.t("error" + string(p.info.NoHunks))
	}
	return ""
}
