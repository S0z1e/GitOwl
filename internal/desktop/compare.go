package desktop

import (
	"context"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"gitowl/internal/workflow"
)

// compareState is the read-only two-ref comparison that the branch menu
// (and later stash / commit menus) show in the main column.
type compareState struct {
	open    bool
	a, b    string
	titleA  string
	titleB  string
	result  workflow.CompareResult
	cursor  int
	diff    *diffPane
	seq     uint64
	err     error
	loading bool
}

func (u *UI) leaveCompare() {
	u.cmp.seq++
	u.cmp = compareState{seq: u.cmp.seq}
}

func (u *UI) startCompare(a, b string) {
	u.startCompareAs(a, b, a, b)
}

func (u *UI) startCompareAs(a, b, titleA, titleB string) {
	repo := u.repo
	if repo == nil {
		return
	}
	u.leaveStash()
	u.cmp.seq++
	seq := u.cmp.seq
	u.cmp = compareState{open: true, a: a, b: b, titleA: titleA, titleB: titleB, seq: seq, cursor: -1, loading: true}
	u.background(func() func() {
		res, err := repo.CompareRefs(context.Background(), a, b)
		return func() {
			if seq != u.cmp.seq || repo != u.repo {
				return
			}
			u.cmp.loading, u.cmp.result, u.cmp.err = false, res, err
			if err == nil && len(res.Files) > 0 {
				u.selectCompareFile(0)
			}
		}
	})
}

func (u *UI) selectCompareFile(i int) {
	if i < 0 || i >= len(u.cmp.result.Files) {
		return
	}
	u.cmp.cursor = i
	u.loadCompareDiff(u.cmp.result.Files[i])
}

func (u *UI) loadCompareDiff(f workflow.Change) {
	repo := u.repo
	if repo == nil {
		return
	}
	u.cmp.seq++
	seq, cmp := u.cmp.seq, u.cmp.result
	key := changeKey{area: workflow.InCompare, path: f.Path}
	u.cmp.diff = &diffPane{key: key, caretHunk: -1, scrollHunk: -1, loading: true}
	u.background(func() func() {
		info, err := repo.CompareFileDiff(context.Background(), cmp, f)
		var state *editor.DiffState
		if err == nil && info.HasText() {
			state, err = diffState(info.Patch, f.Path)
		}
		return func() {
			if seq != u.cmp.seq || repo != u.repo {
				return
			}
			u.cmp.diff = &diffPane{key: key, info: info, state: state, err: err, caretHunk: -1, scrollHunk: -1}
		}
	})
}

func (u *UI) chromeCompare(c *ui.Context) {
	tk := kitDesignTokens(c.Theme().Dark)
	ui.Column(c).Fill().MinWidth(0).Background(tk.bg).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Padding(12, 14, 8).Gap(8).Children(func() {
			a, b := u.cmp.titleA, u.cmp.titleB
			if a == "" {
				a = u.cmp.a
			}
			if b == "" {
				b = u.cmp.b
			}
			if b == "" {
				b = u.t("workingDirectory")
			}
			ui.Text(c, u.t("compareTitle", "a", a, "b", b)).
				Bold().FontSize(16).TextColor(tk.text).SingleLine().Ellipsis("…")
			u.chromeCompareControls(c, tk)
		})
		if u.cmp.err != nil {
			ui.Text(c, u.t("error"+string(noticeOf(u.cmp.err).code))).Padding(12, 14).TextColor(tk.del).Wrap()
			return
		}
		ui.Row(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).Width(260).FillHeight().MinWidth(0).Border(1, tk.border).Children(func() {
				u.chromeCompareFiles(c, tk)
			})
			ui.Column(c).Grow(1).FillHeight().MinWidth(0).Children(func() {
				if p := u.cmp.diff; p != nil {
					u.diffBodyDesign(c, p)
				}
			})
		})
	})
}

func (u *UI) chromeCompareControls(c *ui.Context, tk kitTokens) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Wrap().Children(func() {
		mode := 0
		if u.diffSplitActive() {
			mode = 1
		}
		seg := ui.Segmented(c, &mode, u.t("diffUnified"), u.t("diffSplit"))
		u.chromeMarkFocus(seg)
		if !u.lay.narrow {
			u.diffSplit = mode == 1
		}
		wrap := kitSecondaryButton(c, u.t("diffWrapLines"), tk, false).Height(24)
		wrap.Tooltip(u.t("diffWrapLinesHint"))
		u.chromeMarkFocus(wrap)
		if wrap.Clicked() {
			u.diffWrap = !u.diffWrap
		}
	})
}

func (u *UI) chromeCompareFiles(c *ui.Context, tk kitTokens) {
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(6).Gap(2).Children(func() {
			for i, f := range u.cmp.result.Files {
				u.chromeCompareFileRow(c, tk, i, f)
			}
		})
	})
}

func (u *UI) chromeCompareFileRow(c *ui.Context, tk kitTokens, i int, f workflow.Change) {
	u.chromeCompareFileRowFor(c, tk, i, f, i == u.cmp.cursor, func() { u.selectCompareFile(i) })
}

func (u *UI) chromeCompareFileRowFor(c *ui.Context, tk kitTokens, i int, f workflow.Change, selected bool, choose func()) {
	bg := ui.Color{}
	if selected {
		bg = tk.sel
	}
	dir, file := path.Split(f.Path)
	row := ui.ButtonBase(c).FillWidth().Height(chromeRowHeight).Padding(0, 8).Gap(8).
		Radius(8).Background(bg).Label(file).AlignItems(ui.Center)
	row.Children(func() {
		ui.Text(c, f.Letter()).Font("monospace").FontSize(12).FontWeight(600).TextColor(tk.muted).Width(12)
		ui.Text(c, file).FontSize(13).TextColor(tk.text).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		if dir != "" {
			ui.Text(c, strings.TrimSuffix(dir, "/")).FontSize(12).TextColor(tk.muted).
				SingleLine().Ellipsis("…").Shrink(1000).MinWidth(0)
		}
	})
	if row.Clicked() {
		choose()
	}
}
