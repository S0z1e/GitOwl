package desktop

import (
	"errors"
	"gitowl/internal/desktop/designicons"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// chromeHistoryListHeight is the commit list's height while the detail is
// open: the History board's 312 DIP list above the detail panel.
const chromeHistoryListHeight = 312

func (u *UI) chromeHistoryCount() int {
	if u.repo == nil || u.snap.Unborn() {
		return 0
	}
	return len(u.hist.commits)
}

// chromeHistory is All commits: the toolbar, the full list on top and a
// closable detail under it. The list is one Graph of the shown branches, or
// the results of the search box, read in one piece with no pagination.
func (u *UI) chromeHistory(c *ui.Context) {
	tk := kitDesignTokens(c.Theme().Dark)
	u.chromeEnsureHistorySelection()
	ui.Column(c).Fill().MinWidth(0).Background(tk.bg).Children(func() {
		if u.repo != nil && !u.snap.Unborn() {
			u.chromeHistoryBar(c, tk)
		}
		ui.Column(c).Grow(1).FillWidth().MinWidth(0).MinHeight(0).Children(func() {
			u.chromeHistoryBody(c, tk)
		})
	})
}

// chromeSearchPage is the Search page: the criterion, text and branch scope
// in its toolbar, then the matching commits as a flat list with the detail
// panel under it. Before anything is typed it shows a hint, not the list.
func (u *UI) chromeSearchPage(c *ui.Context) {
	tk := kitDesignTokens(c.Theme().Dark)
	ui.Column(c).Fill().MinWidth(0).Background(tk.bg).Children(func() {
		if u.repo != nil && !u.snap.Unborn() {
			u.chromeSearchBar(c, tk)
		}
		ui.Column(c).Grow(1).FillWidth().MinWidth(0).MinHeight(0).Children(func() {
			if strings.TrimSpace(u.hist.text) == "" {
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, u.t("searchEmptyHint")).FontSize(13).TextColor(tk.muted).Wrap()
				})
				return
			}
			u.chromeEnsureHistorySelection()
			u.chromeHistoryBody(c, tk)
		})
	})
}

// chromeHistoryBody draws what the list area shows: the failure with a retry,
// a search still reading, a search with no match, an empty repository, or the
// list and its detail.
func (u *UI) chromeHistoryBody(c *ui.Context, tk kitTokens) {
	switch {
	case u.hist.err != nil && len(u.hist.commits) == 0:
		msg, retry := u.historyFailure()
		ui.Column(c).Fill().Center().Gap(10).Children(func() {
			ui.Text(c, msg).TextColor(tk.del).Wrap()
			if retry && kitSecondaryButton(c, u.t("historyRetry"), tk, false).Clicked() {
				u.historyChanged()
			}
		})
	case !u.hist.loaded && !u.snap.Unborn():
		// A read is under way: the first visit, or the graph is being drawn
		// again for a new branch, sort, Remote branches choice or search.
		ui.Column(c).Fill().Center().Gap(10).Children(func() {
			kitSpinner(c, tk, 16)
			ui.Text(c, u.t("historyLoading")).FontSize(13).TextColor(tk.muted)
		})
	case u.hist.searching && len(u.hist.commits) == 0:
		ui.Column(c).Fill().Center().Gap(6).Children(func() {
			ui.Text(c, u.t("noMatchesTitle")).FontSize(14).FontWeight(600).TextColor(tk.text)
			ui.Text(c, u.t("noMatchesBody", "scope", u.historyScopeText(), "text", strings.TrimSpace(u.hist.text))).
				FontSize(13).TextColor(tk.muted).Wrap()
		})
	case u.snap.Unborn() || len(u.hist.commits) == 0:
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, u.t("noCommitsYet")).FontSize(14).TextColor(tk.muted)
		})
	default:
		u.chromeHistoryPanels(c, tk)
	}
}

// historyFailure says why the list could not be read, and whether a retry
// can help. A search the user fixes by typing differently needs no retry.
func (u *UI) historyFailure() (msg string, retry bool) {
	var e *workflow.Error
	if errors.As(u.hist.err, &e) {
		switch e.Code {
		case workflow.CodeInvalidHash:
			return u.t("errorInvalidHash"), false
		case workflow.CodeScopeMissing:
			return u.t("errorScopeMissing", "scope", e.Detail), false
		}
	}
	return u.t("historyFailed"), true
}

// chromeHistoryPanels draws the commit list and, under it, the detail. The
// detail slides open and shut over detailSlide: its progress sets the list's
// height between the list's own height and the whole body, and the detail
// takes what the list leaves. A closed detail that is still sliding shut is
// built until it is gone.
func (u *UI) chromeHistoryPanels(c *ui.Context, tk kitTokens) {
	h := &u.hist
	body := ui.Column(c).FillWidth().Grow(1).MinHeight(0)
	target := float32(0)
	if h.detailOpen {
		target = 1
	}
	p := body.Animate("detail", target, detailSlide)
	body.Children(func() {
		if !h.detailOpen && p == 0 {
			ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(8, 8, 0, 8).Children(func() {
				u.chromeCommitList(c, tk)
			})
			return
		}
		// The list keeps its height while open, and starts from the whole
		// body when the slide begins, as the body was last frame.
		listH := u.histListHeight()
		from := max(h.bodyH, listH)
		listH += (from - listH) * (1 - p)
		ui.Column(c).FillWidth().Height(listH).Shrink(0).Padding(8, 8, 0, 8).Children(func() {
			u.chromeCommitList(c, tk)
		})
		u.chromeCommitDetail(c, tk)
	})
	h.bodyH = body.Bounds().H
}

func (u *UI) chromeEnsureHistorySelection() {
	h := &u.hist
	if h.loading || len(h.commits) == 0 {
		return
	}
	if h.selected == "" {
		h.detailOpen = true
		u.chromeOpenCommit(0)
		return
	}
	if h.detail == nil && !h.detailLoading {
		u.chromeOpenCommit(h.cursor)
	}
}

func (u *UI) chromeOpenCommit(i int) {
	h := &u.hist
	if i < 0 || i >= len(h.commits) {
		return
	}
	h.detailTab = 0
	h.detailOpen = true
	u.openCommit(i)
}

// chromeCommitList draws the commit rows in a virtual list, so a long history
// builds only the rows on screen and scrolls smoothly.
func (u *UI) chromeCommitList(c *ui.Context, tk kitTokens) {
	h := &u.hist
	if h.scrollTo >= 0 && h.scrollTo < len(h.commits) {
		h.commitList.ScrollIntoView(h.scrollTo)
		h.scrollTo = -1
	}
	h.commitList.Key = func(i int) any { return h.commits[i].Hash }
	ui.List(c, &h.commitList, len(h.commits), func(i int) {
		u.chromeCommitRow(c, tk, i)
	}).Fill()
}

func (u *UI) chromeCommitRow(c *ui.Context, tk kitTokens, i int) {
	h := &u.hist
	cm := h.commits[i]
	bg := ui.Color{}
	if i == h.cursor {
		bg = tk.sel
	}
	// One line per commit: the Graph slot, then the subject with its ref
	// pills, the author, the short hash and the date, as the board draws it.
	row := ui.ButtonBase(c).FillWidth().Height(commitRowHeight).Padding(0, 14, 0, 0).
		Background(bg).Label(cm.Subject).AlignItems(ui.Center)
	row.ContextMenu(func(m *ui.Menu) { u.chromeCommitItems(m, cm) })
	u.focusReturn(row, "commit:"+cm.Hash)
	row.Children(func() {
		// The Search page's rows are a flat list: no Graph lanes or edges.
		if u.middleTab != 2 {
			u.graphSlot(c, i)
		}
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Clip().Children(func() {
				// Refs lead the subject, as the design's row does.
				for _, l := range u.graphLabels(cm.Hash) {
					u.chromeRefPill(c, tk, l)
				}
				u.chromeClassPill(c, tk, u.classAt(i))
				ui.Text(c, cm.Subject).FontSize(13).TextColor(tk.text).
					SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			})
			ui.Row(c).Width(110).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Box(c).Size(18, 18).Radius(9).Background(tk.groupColor(authorIndex(cm))).Shrink(0)
				ui.Text(c, cm.Author).FontSize(12).TextColor(tk.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			})
			ui.Text(c, cm.Short()).Font("monospace").FontSize(12).TextColor(tk.muted).Width(64).Shrink(0)
			ui.Row(c).Width(128).Justify(ui.End).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Text(c, u.t("commitWhen", "when", cm.When)).FontSize(12).TextColor(tk.muted).SingleLine()
			})
		})
	})
	if row.Clicked() {
		u.chromeOpenCommit(i)
	}
}

// chromeRefPill draws a ref as the design's .ref: a 20-tall, 7-radius pill in
// the ref tint, with a branch, cloud (remote) or tag icon before its name.
func (u *UI) chromeRefPill(c *ui.Context, tk kitTokens, l commitLabel) {
	icon := designicons.Branch()
	switch {
	case l.remote:
		icon = designicons.Cloud()
	case l.tag:
		icon = designicons.Tag()
	}
	// A branch takes the colour of its branch group, as its lanes do; a tag
	// keeps the design's neutral ref tint.
	bg, bd, fg := tk.refbg, tk.refbd, tk.text
	if !l.tag {
		group := u.labelColour(tk, l)
		bg, bd = group.Alpha(.14), group.Alpha(.45)
		fg = group
	}
	ui.Row(c).Shrink(0).MaxWidth(180).Height(20).Padding(0, 7).Gap(5).Radius(7).
		Background(bg).Border(1, bd).AlignItems(ui.Center).Tooltip(l.name).Children(func() {
		ui.Icon(c, icon).Size(11, 11).TextColor(fg).Shrink(0)
		ui.Text(c, l.name).FontSize(12).FontWeight(500).TextColor(tk.text).SingleLine().Ellipsis("…")
	})
}

func (u *UI) chromeClassPill(c *ui.Context, tk kitTokens, class workflow.GraphClass) {
	var label string
	switch class {
	case workflow.Unpushed:
		label = u.t("graphNotPushed")
	case workflow.Incoming:
		label = u.t("graphIncoming")
	default:
		return
	}
	ui.Row(c).Shrink(0).Padding(2, 7).Radius(999).Background(tk.fill).Border(1, tk.bd2).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Font("monospace").FontSize(11).TextColor(tk.muted)
	})
}

func (u *UI) chromeCopyHash(c *ui.Context, tk kitTokens, hash string) {
	b := ui.ButtonBase(c).Size(22, 22).Label(u.t("historyCopyHash"))
	b.Children(func() {
		ui.Icon(c, designicons.Copy()).Size(14, 14).TextColor(tk.muted)
	})
	if hash != "" && b.Clicked() {
		c.WriteClipboard(hash)
		if c.ReadClipboard() == hash {
			u.toast("success", u.t("historyHashCopied"))
		} else {
			u.toast("error", u.t("historyHashCopyFailed"))
		}
	}
}

func (u *UI) chromeCommitDetail(c *ui.Context, tk kitTokens) {
	h := &u.hist
	// The panel fills the height under the list and sits 10 DIP in from the
	// sides and bottom, as the History board's detail section does.
	ui.Column(c).FillWidth().Grow(1).MinHeight(0).Padding(0, 10, 10, 10).Children(func() {
		u.chromeDetailGrip(c, tk)
		panel := ui.Column(c).FillWidth().Grow(1).MinHeight(0).Background(tk.surf).
			Border(1, tk.border).Radius(16).Clip()
		h.panelH = panel.Bounds().H
		if h.detail != nil {
			cm := h.detail.CommitInfo
			panel.ContextMenu(func(m *ui.Menu) { u.chromeCommitItems(m, cm) })
		}
		panel.Children(func() {
			ui.Row(c).FillWidth().Height(46).Padding(0, 10).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Segmented(c, &h.detailTab, u.t("chromeCommitTab"), u.t("chromeChangesTab"), u.t("chromeFilesTab"))
				ui.Box(c).Grow(1)
				close := ui.ButtonBase(c).Size(28, 28).Radius(8).Center().Label(u.t("chromeCloseDetail"))
				if close.Hovered() {
					close.Background(tk.text.Alpha(.05))
				}
				close.Children(func() {
					ui.Icon(c, designicons.Close()).Size(14, 14).TextColor(tk.muted)
				})
				if close.Clicked() {
					h.detailOpen = false
				}
			})
			ui.Box(c).FillWidth().Height(1).Background(tk.border)
			ui.Column(c).Grow(1).MinHeight(0).Padding(14, 20, 14, 20).Children(func() {
				switch {
				case h.detailErr != nil:
					ui.Text(c, h.detailErr.Error()).TextColor(tk.del).Wrap()
				case h.detail == nil:
					return
				case h.detailTab == 1:
					u.chromeHistoryChanges(c, tk)
				case h.detailTab == 2:
					u.chromeHistoryFiles(c, tk, false)
				default:
					u.chromeHistoryCommit(c, tk)
				}
			})
		})
	})
}

// Bounds for the grip above the detail panel: the list keeps at least
// minHistListHeight, and a panel released shorter than detailCloseHeight
// closes.
const (
	minHistListHeight = 120
	detailCloseHeight = 120
	detailGripHeight  = 12
	// detailSlide is how long the detail takes to open or shut.
	detailSlide = 200 * time.Millisecond
)

// histListHeight is the list's height while the detail is open.
func (u *UI) histListHeight() float32 {
	if u.hist.listHeight > 0 {
		return u.hist.listHeight
	}
	return chromeHistoryListHeight
}

// chromeDetailGrip draws the handle above the detail panel. Dragging it down
// takes height from the panel and gives it to the list; releasing it with the
// panel shorter than detailCloseHeight closes the panel, and any other
// release keeps the height the drag reached.
func (u *UI) chromeDetailGrip(c *ui.Context, tk kitTokens) {
	h := &u.hist
	grip := ui.Row(c).FillWidth().Height(detailGripHeight).Shrink(0).Center().Cursor(ui.CursorResizeNS)
	grip.Children(func() {
		ui.Box(c).Size(36, 4).Radius(2).Background(tk.border)
	})
	_, dy, held := grip.Dragged()
	switch {
	case held:
		h.gripping = true
		if dy != 0 {
			h.listHeight = max(u.histListHeight()+dy, minHistListHeight)
		}
	case h.gripping:
		h.gripping = false
		if h.panelH < detailCloseHeight {
			h.detailOpen = false
		}
	}
}

func (u *UI) chromeHistoryCommit(c *ui.Context, tk kitTokens) {
	d := u.hist.detail
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(0).Children(func() {
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(36, 36).Radius(18).Background(tk.groupColor(authorIndex(d.CommitInfo))).Shrink(0)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, d.Author).FontSize(13).FontWeight(700).TextColor(tk.text).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
						ui.Text(c, "·").FontSize(13).TextColor(tk.muted).Shrink(0)
						ui.Text(c, u.t("commitWhen", "when", d.When)).FontSize(12).TextColor(tk.muted).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
					})
				})
			})
			ui.Box(c).FillWidth().Height(1).Background(tk.border).Margin(14, 0, 0, 0)
			ui.Text(c, d.Subject).FontSize(17).FontWeight(700).TextColor(tk.text).Wrap().Margin(10, 0, 8, 0).Selectable()
			if body := commitBody(d.Message, d.Subject); body != "" {
				ui.Text(c, body).FontSize(13).TextColor(tk.text).Wrap().Margin(0, 0, 12, 0)
			}
			ui.Row(c).FillWidth().Gap(24).Wrap().Children(func() {
				u.chromeDetailField(c, tk, u.t("chromeCommitTab"), func() {
					ui.Text(c, d.Hash).Font("monospace").FontSize(12).TextColor(tk.text).Selectable().Wrap()
					u.chromeCopyHash(c, tk, d.Hash)
				})
				if len(d.Parents) > 0 {
					label := u.t("chromeParent")
					if len(d.Parents) > 1 {
						label = u.t("chromeParents")
					}
					u.chromeDetailField(c, tk, label, func() {
						for _, p := range d.Parents {
							u.chromeParentHash(c, tk, p)
						}
					})
				}
			})
			ui.Row(c).FillWidth().Gap(24).Wrap().Margin(6, 0, 0, 0).Children(func() {
				if refs := u.graphLabels(d.Hash); len(refs) > 0 {
					// No label: the locale has no "Refs" string yet, and the
					// pills read as refs on their own.
					u.chromeDetailField(c, tk, "", func() {
						for _, l := range refs {
							u.chromeRefPill(c, tk, l)
						}
					})
				}
				if len(d.Files) > 0 {
					added, removed := commitLineTotals(d.Files)
					u.chromeDetailField(c, tk, u.t("chromeFilesTab"), func() {
						ui.Text(c, u.t("changesCount", "count", len(d.Files))).FontSize(13).TextColor(tk.text)
						ui.Text(c, "+"+strconv.Itoa(added)).Font("monospace").FontSize(12).TextColor(tk.add)
						ui.Text(c, "−"+strconv.Itoa(removed)).Font("monospace").FontSize(12).TextColor(tk.del)
					})
				}
			})
			if len(d.Files) > 0 {
				ui.Text(c, strings.ToUpper(u.t("changesCount", "count", len(d.Files)))).FontSize(11).FontWeight(600).
					LetterSpacing(.05).TextColor(tk.muted).Margin(16, 0, 6, 0)
			}
			u.chromeHistoryFiles(c, tk, true)
		})
	})
}

// chromeDetailField draws one labelled value of the commit detail: a muted
// label of the design's 64 DIP width and the value beside it.
func (u *UI) chromeDetailField(c *ui.Context, tk kitTokens, label string, value func()) {
	ui.Row(c).Grow(1).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(tk.muted).Width(64).Shrink(0)
		ui.Row(c).Grow(1).MinWidth(0).Gap(6).AlignItems(ui.Center).Wrap().Children(value)
	})
}

// commitLineTotals sums the added and removed lines of a commit's text
// files, the figures the Files field shows.
func commitLineTotals(files []workflow.Change) (added, removed int) {
	for _, f := range files {
		if !f.Binary {
			added += f.Added
			removed += f.Removed
		}
	}
	return added, removed
}

func (u *UI) chromeParentHash(c *ui.Context, tk kitTokens, hash string) {
	short := hash
	if len(short) > 7 {
		short = short[:7]
	}
	i := u.chromeHistoryIndex(hash)
	if i < 0 {
		ui.Text(c, short).Font("monospace").FontSize(12).TextColor(tk.muted)
		return
	}
	b := ui.ButtonBase(c).Label(short)
	b.Children(func() {
		ui.Text(c, short).Font("monospace").FontSize(12).TextColor(tk.accent)
	})
	if b.Clicked() {
		u.hist.scrollTo = i
		u.chromeOpenCommit(i)
	}
}

func (u *UI) chromeHistoryIndex(hash string) int {
	for i, c := range u.hist.commits {
		if c.Hash == hash {
			return i
		}
	}
	return -1
}

func (u *UI) chromeHistoryFiles(c *ui.Context, tk kitTokens, withStats bool) {
	d := u.hist.detail
	if d == nil {
		return
	}
	if len(d.Files) == 0 {
		ui.Text(c, u.t("noFilesInCommit")).FontSize(12).TextColor(tk.muted)
		return
	}
	ui.Column(c).FillWidth().Gap(2).Children(func() {
		for i, f := range d.Files {
			u.chromeHistoryFileRow(c, tk, i, f, withStats)
		}
	})
}

func (u *UI) chromeHistoryFileRow(c *ui.Context, tk kitTokens, i int, f workflow.Change, withStats bool) {
	bg := ui.Color{}
	if i == u.hist.fileCursor {
		bg = tk.sel
	}
	dir, file := path.Split(f.Path)
	row := ui.ButtonBase(c).FillWidth().Height(38).Padding(0, 10).Gap(10).
		Radius(10).Background(bg).Label(file).AlignItems(ui.Center).Justify(ui.Start)
	if u.opts.Launch != nil {
		row.ContextMenu(func(m *ui.Menu) {
			m.Item(u.t("currentVersionNote")).Disabled(true)
			u.fileItems(m, f.Path, u.chromeHistoryFileMissing(f.Path))
		})
	}
	row.Children(func() {
		chromeStatusBadge(c, tk, f.Letter())
		ui.Text(c, file).FontSize(13).FontWeight(500).TextColor(tk.text).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		if dir != "" {
			ui.Text(c, strings.TrimSuffix(dir, "/")).FontSize(12).TextColor(tk.muted).
				SingleLine().Ellipsis("…").Shrink(1000).MinWidth(0)
		}
		if withStats && !f.Binary {
			ui.Row(c).Gap(6).Shrink(0).Children(func() {
				ui.Text(c, "+"+strconv.Itoa(f.Added)).Font("monospace").FontSize(12).TextColor(tk.add).Shrink(0)
				ui.Text(c, "−"+strconv.Itoa(f.Removed)).Font("monospace").FontSize(12).TextColor(tk.del).Shrink(0)
			})
		}
	})
	if row.Clicked() {
		u.chromeSelectHistoryFile(i)
	}
}

func (u *UI) chromeHistoryFileMissing(rel string) bool {
	if u.repo == nil || rel == "" {
		return true
	}
	_, err := os.Stat(filepath.Join(u.repo.Root(), rel))
	return err != nil
}

func (u *UI) chromeSelectHistoryFile(i int) {
	u.hist.detailTab = 1
	u.selectCommitFile(i)
}

func (u *UI) chromeHistoryChanges(c *ui.Context, tk kitTokens) {
	h := &u.hist
	if h.fileCursor < 0 || h.diff == nil {
		ui.Text(c, u.t("chromePickFile")).FontSize(13).TextColor(tk.muted)
		return
	}
	ui.Column(c).FillWidth().MinHeight(120).Children(func() {
		u.diffBodyDesign(c, h.diff)
	})
}
