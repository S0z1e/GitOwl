package desktop

import (
	"strconv"

	"gitowl/internal/desktop/designicons"
	"gitowl/internal/workflow"

	"github.com/egoist/mygo/ui"
)

// chromeHistoryPanelListH is the most the branch dropdown's list shows before
// it scrolls: about ten rows, so a long branch list stays inside the window.
const chromeHistoryPanelListH = 340

// chromeHistoryBar is the toolbar above the History graph, the design's filter
// bar: the branch dropdown, the Remote branches switch and the sort dropdown.
// It sits on the list's own row, with a hairline under it.
func (u *UI) chromeHistoryBar(c *ui.Context, tk kitTokens) {
	u.chromeBarFrame(c, tk, nil)
}

// chromeSearchBar is the toolbar of the Search page: the search box in front
// of the same branch, Remote branches and sort controls the History bar has.
func (u *UI) chromeSearchBar(c *ui.Context, tk kitTokens) {
	u.chromeBarFrame(c, tk, func() { u.chromeHistorySearch(c, tk) })
}

// chromeBarFrame draws a toolbar row with lead at its start, then the branch,
// Remote branches and sort controls at its end, over a hairline.
func (u *UI) chromeBarFrame(c *ui.Context, tk kitTokens, lead func()) {
	ui.Column(c).FillWidth().Shrink(0).Children(func() {
		ui.Row(c).FillWidth().Padding(10, 14, 10, 14).Gap(8).AlignItems(ui.Center).Children(func() {
			if lead != nil {
				lead()
			}
			ui.Box(c).Grow(1)
			u.chromeHistoryBranches(c, tk)
			u.chromeHistoryRemotes(c, tk)
			u.chromeHistorySort(c, tk)
		})
		ui.Box(c).FillWidth().Height(1).Background(tk.bd3)
	})
}

// chromeHistorySearch is the History search: a criterion (commit message,
// commit hash or author) chosen in its menu, and the text it matches. Each
// criterion looks at one thing only, so a word in a message never matches
// an author.
func (u *UI) chromeHistorySearch(c *ui.Context, tk kitTokens) {
	h := &u.hist
	ui.Row(c).Grow(1).MinWidth(0).MaxWidth(420).Height(kitToolbarH).Radius(11).Gap(0).
		Background(tk.surf).Border(1, tk.bd2).Padding(0, 6, 0, 12).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, designicons.Search()).Size(14, 14).TextColor(tk.muted).Shrink(0)
		trigger := ui.ButtonBase(c).Height(kitToolbarH).Padding(0, 8, 0, 8).Gap(6).Radius(8).
			AlignItems(ui.Center).Shrink(0).Label(u.historyFieldText(h.field))
		trigger.Children(func() {
			ui.Text(c, u.historyFieldText(h.field)).FontSize(13).FontWeight(500).TextColor(tk.text).SingleLine()
			ui.Icon(c, designicons.ChevronDown()).Size(12, 12).TextColor(tk.muted)
		})
		if trigger.Clicked() {
			h.pop.field = !h.pop.field
		}
		overlayPopover(c, trigger, &h.pop.field, func() {
			u.chromeHistoryFieldPanel(c, tk)
		})
		ui.Box(c).Size(1, 16).Background(tk.bd2).Shrink(0)
		field := ui.TextInputBase(c, &h.text).Grow(1).MinWidth(0).FillWidth().Padding(0, 8, 0, 8).
			Label(u.t("searchHistory")).Placeholder(u.historyFieldPlaceholder(h.field))
		u.chromeMarkFocus(field)
		if field.Changed() {
			u.applySearch()
		}
	})
}

// chromeHistoryFieldPanel is the search criterion menu: one radio choice per
// criterion. Picking one searches again with the text already typed.
func (u *UI) chromeHistoryFieldPanel(c *ui.Context, tk kitTokens) {
	h := &u.hist
	ui.Column(c).Width(200).Gap(0).Radius(16).Background(tk.surf).Border(1, tk.border).Clip().
		Padding(6, 8, 6, 8).Children(func() {
		for _, f := range workflow.HistoryFields {
			u.chromeHistoryChoice(c, tk, u.historyFieldText(f), h.field == f, func() {
				h.field = f
				h.pop.field = false
				u.applySearch()
			})
		}
	})
}

// chromeHistoryBranches is the branch dropdown's trigger. Its label is the
// picked branch, or All branches.
func (u *UI) chromeHistoryBranches(c *ui.Context, tk kitTokens) {
	h := &u.hist
	icon := designicons.Branch()
	label := u.t("historyAllBranches")
	if p, ok := u.picked(); ok {
		label = p.Name
		if p.Remote {
			icon = designicons.Cloud()
		}
	}
	trigger := ui.ButtonBase(c).Height(kitToolbarH).Padding(0, 14).Gap(8).Radius(11).
		Background(tk.surf).Border(1, tk.bd2).Label(label).AlignItems(ui.Center).Shrink(0)
	trigger.Children(func() {
		ui.Icon(c, icon).Size(14, 14).TextColor(tk.muted)
		ui.Text(c, label).FontWeight(500).TextColor(tk.text).SingleLine().Ellipsis("…").MaxWidth(220)
		ui.Icon(c, designicons.ChevronDown()).Size(12, 12).TextColor(tk.muted)
	})
	if trigger.Clicked() {
		h.pop.branches = !h.pop.branches
	}
	overlayPopover(c, trigger, &h.pop.branches, func() {
		u.chromeHistoryBranchPanel(c, tk)
	})
}

// chromeHistoryBranchPanel is the branch dropdown: a filter, then All branches,
// the current branch, the other local branches and, while Remote branches is
// on, one group per remote. Picking one shows only that branch. Only the list
// scrolls, so the filter stays in view.
func (u *UI) chromeHistoryBranchPanel(c *ui.Context, tk kitTokens) {
	h := &u.hist
	v := u.branchView()
	total := len(u.historyBranches())
	if !h.remotes {
		total = 0
		for _, s := range u.historyBranches() {
			if !s.Remote {
				total++
			}
		}
	}
	ui.Column(c).Width(330).Gap(0).Radius(16).Background(tk.surf).Border(1, tk.border).Clip().Children(func() {
		ui.Row(c).FillWidth().Padding(12, 14, 2, 14).Children(func() {
			ui.Row(c).Grow(1).Height(kitFieldH).Radius(10).Gap(8).Background(tk.surf).Border(1, tk.bd2).
				Padding(0, 10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, designicons.Search()).Size(14, 14).TextColor(tk.muted).Shrink(0)
				field := ui.TextInputBase(c, &h.branchFilter).Grow(1).MinWidth(0).FillWidth().
					Label(u.t("historyBranchFilter")).Placeholder(u.t("historyBranchFilter"))
				u.chromeMarkFocus(field)
			})
		})
		ui.Scroll(c).FillWidth().MaxHeight(chromeHistoryPanelListH).Children(func() {
			ui.Column(c).FillWidth().Padding(4, 8, 8, 8).Children(func() {
				u.chromeHistoryOption(c, tk, designicons.Branch(), u.t("historyAllBranches"),
					strconv.Itoa(total), !v.picked, func() { u.pickBranch("") })
			})
			u.chromeHistoryCurrentSection(c, tk, v)
			u.chromeHistoryOtherSection(c, tk, v)
			if h.remotes {
				u.chromeHistoryRemoteSections(c, tk, v)
			}
		})
	})
}

// chromeHistoryLabel is a dropdown section's label at the design's 18 DIP inset.
func chromeHistoryLabel(c *ui.Context, tk kitTokens, label string) {
	ui.Box(c).FillWidth().Padding(10, 18, 4, 18).Children(func() {
		chromeSyncSection(c, tk, label)
	})
}

// chromeHistoryOption is one choice of the branch dropdown: a radio circle, the
// branch's icon and name, and a meta text. The picked one is tinted and bold.
func (u *UI) chromeHistoryOption(c *ui.Context, tk kitTokens, icon *ui.SVG, label, meta string, on bool, pick func()) {
	row := ui.ButtonBase(c).FillWidth().Height(32).Padding(0, 10).Gap(10).Radius(9).
		Justify(ui.Start).AlignItems(ui.Center).Label(label)
	if on {
		row.Background(tk.accent.Alpha(.12))
	}
	row.Children(func() {
		chromeRadioFace(c, on, tk)
		ui.Icon(c, icon).Size(14, 14).TextColor(tk.muted).Shrink(0)
		weight := 500
		if on {
			weight = 600
		}
		ui.Text(c, label).FontSize(13).FontWeight(weight).TextColor(tk.text).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		if meta != "" {
			ui.Box(c).Grow(1)
			ui.Text(c, meta).FontSize(12).TextColor(tk.muted).SingleLine()
		}
	})
	if row.Clicked() && !on {
		pick()
	}
}

// chromeHistoryCurrentSection lists the checked-out branch, when there is one
// and it matches the filter.
func (u *UI) chromeHistoryCurrentSection(c *ui.Context, tk kitTokens, v branchView) {
	h := &u.hist
	if u.snap.Branch == "" || !matchFilter(u.snap.Branch, h.branchFilter) {
		return
	}
	ref := "refs/heads/" + u.snap.Branch
	chromeHistoryLabel(c, tk, u.t("historyCurrentBranch"))
	ui.Column(c).FillWidth().Padding(0, 8, 0, 8).Children(func() {
		u.chromeHistoryOption(c, tk, designicons.Branch(), u.snap.Branch, "", v.pickRef() == ref, func() {
			u.pickBranch(ref)
		})
	})
}

// chromeHistoryOtherSection lists the local branches other than the checked-out
// one that match the filter.
func (u *UI) chromeHistoryOtherSection(c *ui.Context, tk kitTokens, v branchView) {
	h := &u.hist
	var rows []workflow.HistoryScope
	for _, s := range u.historyBranches() {
		if !s.Remote && s.Name != u.snap.Branch && matchFilter(s.Name, h.branchFilter) {
			rows = append(rows, s)
		}
	}
	if len(rows) == 0 {
		return
	}
	chromeHistoryLabel(c, tk, u.t("historyOtherBranches"))
	ui.Column(c).FillWidth().Padding(0, 8, 0, 8).Children(func() {
		for _, s := range rows {
			u.chromeHistoryBranchOption(c, tk, s, v)
		}
	})
}

// chromeHistoryRemoteSections lists the remote sources, each under its own
// name, under one "Remote" label.
func (u *UI) chromeHistoryRemoteSections(c *ui.Context, tk kitTokens, v branchView) {
	h := &u.hist
	var groups []remoteSource
	for _, src := range u.remoteSources() {
		for _, s := range src.branches {
			if matchFilter(s.Name, h.branchFilter) {
				groups = append(groups, src)
				break
			}
		}
	}
	if len(groups) == 0 {
		return
	}
	chromeHistoryLabel(c, tk, u.t("historyRemote"))
	for _, src := range groups {
		ui.Box(c).FillWidth().Padding(4, 18, 2, 18).Children(func() {
			ui.Text(c, src.name).FontSize(12).FontWeight(600).TextColor(tk.muted).SingleLine()
		})
		ui.Column(c).FillWidth().Padding(0, 8, 0, 8).Children(func() {
			for _, s := range src.branches {
				if matchFilter(s.Name, h.branchFilter) {
					u.chromeHistoryBranchOption(c, tk, s, v)
				}
			}
		})
	}
}

// chromeHistoryBranchOption is one branch of the dropdown, with its icon and
// the picked tint.
func (u *UI) chromeHistoryBranchOption(c *ui.Context, tk kitTokens, s workflow.HistoryScope, v branchView) {
	icon := designicons.Branch()
	if s.Remote {
		icon = designicons.Cloud()
	}
	u.chromeHistoryOption(c, tk, icon, s.Name, "", v.pickRef() == s.Ref, func() {
		u.pickBranch(s.Ref)
	})
}

// chromeHistoryRemotes is the Remote branches switch: a 34 × 20 track with a
// knob that sits at the right when on. Turning it off drops a picked remote
// branch, which the toolbar would otherwise keep showing.
func (u *UI) chromeHistoryRemotes(c *ui.Context, tk kitTokens) {
	h := &u.hist
	label := u.t("historyRemoteBranches")
	row := ui.ButtonBase(c).Height(kitToolbarH).Padding(0, 4).Gap(8).Radius(9).
		Justify(ui.Start).AlignItems(ui.Center).Shrink(0).Label(label)
	row.Children(func() {
		track, justify := tk.bd2, ui.Start
		if h.remotes {
			track, justify = tk.accent, ui.End
		}
		ui.Row(c).Size(34, 20).Radius(10).Padding(2).Background(track).Justify(justify).
			AlignItems(ui.Center).Shrink(0).Children(func() {
			ui.Box(c).Size(16, 16).Radius(8).Background(tk.surf).Shrink(0)
		})
		ui.Text(c, label).FontSize(13).TextColor(tk.text).SingleLine()
	})
	if row.Clicked() {
		h.remotes = !h.remotes
		if p, ok := u.picked(); ok && p.Remote && !h.remotes {
			h.shownRef = ""
		}
		u.applySearch()
	}
}

// chromeHistorySort is the sort dropdown: its label shows the order, and its
// popover picks the order and the date the list sorts by.
func (u *UI) chromeHistorySort(c *ui.Context, tk kitTokens) {
	h := &u.hist
	label := u.t("historyNewestFirst")
	if h.order.Oldest {
		label = u.t("historyOldestFirst")
	}
	trigger := ui.ButtonBase(c).Height(kitToolbarH).Padding(0, 14).Gap(8).Radius(11).
		Background(tk.surf).Border(1, tk.bd2).Label(label).AlignItems(ui.Center).Shrink(0)
	trigger.Children(func() {
		ui.Icon(c, designicons.Sort()).Size(14, 14).TextColor(tk.muted)
		ui.Text(c, label).FontWeight(500).TextColor(tk.text).SingleLine()
		ui.Icon(c, designicons.ChevronDown()).Size(12, 12).TextColor(tk.muted)
	})
	if trigger.Clicked() {
		h.pop.sort = !h.pop.sort
	}
	overlayPopover(c, trigger, &h.pop.sort, func() {
		u.chromeHistorySortPanel(c, tk)
	})
}

// chromeHistorySortPanel is the sort dropdown: Order (newest or oldest first)
// and Sort by (commit date, author date or topological).
func (u *UI) chromeHistorySortPanel(c *ui.Context, tk kitTokens) {
	h := &u.hist
	ui.Column(c).Width(250).Gap(0).Radius(16).Background(tk.surf).Border(1, tk.border).Clip().
		Padding(0, 0, 10, 0).Children(func() {
		chromeHistoryLabel(c, tk, u.t("historyOrder"))
		ui.Column(c).FillWidth().Padding(0, 8, 0, 8).Children(func() {
			u.chromeHistoryChoice(c, tk, u.t("historyNewestFirst"), !h.order.Oldest, func() {
				h.order.Oldest = false
				u.applySearch()
			})
			u.chromeHistoryChoice(c, tk, u.t("historyOldestFirst"), h.order.Oldest, func() {
				h.order.Oldest = true
				u.applySearch()
			})
		})
		chromeHistoryLabel(c, tk, u.t("historySortBy"))
		ui.Column(c).FillWidth().Padding(0, 8, 0, 8).Children(func() {
			keys := []struct {
				key   workflow.SortKey
				label string
			}{
				{workflow.SortCommitDate, u.t("historySortCommitDate")},
				{workflow.SortAuthorDate, u.t("historySortAuthorDate")},
				{workflow.SortTopo, u.t("historySortTopo")},
			}
			for _, k := range keys {
				u.chromeHistoryChoice(c, tk, k.label, h.order.Key == k.key, func() {
					h.order.Key = k.key
					u.applySearch()
				})
			}
		})
	})
}

// chromeHistoryChoice is one option of the sort dropdown: a radio circle and
// its label, the chosen one tinted and bold.
func (u *UI) chromeHistoryChoice(c *ui.Context, tk kitTokens, label string, on bool, pick func()) {
	row := ui.ButtonBase(c).FillWidth().Height(32).Padding(0, 10).Gap(10).Radius(9).
		Justify(ui.Start).AlignItems(ui.Center).Label(label)
	if on {
		row.Background(tk.accent.Alpha(.12))
	}
	row.Children(func() {
		chromeRadioFace(c, on, tk)
		weight := 500
		if on {
			weight = 600
		}
		ui.Text(c, label).FontSize(13).FontWeight(weight).TextColor(tk.text).SingleLine()
	})
	if row.Clicked() && !on {
		pick()
	}
}
